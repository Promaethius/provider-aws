/*
Copyright 2020 The Crossplane Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package nodegroup

import (
	"context"
	"net/http"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	awseks "github.com/aws/aws-sdk-go-v2/service/eks"
	"github.com/google/go-cmp/cmp"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	runtimev1alpha1 "github.com/crossplane/crossplane-runtime/apis/core/v1alpha1"
	"github.com/crossplane/crossplane-runtime/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/pkg/resource"
	"github.com/crossplane/crossplane-runtime/pkg/test"

	"github.com/crossplane/provider-aws/apis/eks/v1alpha1"
	awsv1alpha3 "github.com/crossplane/provider-aws/apis/v1alpha3"
	awsclients "github.com/crossplane/provider-aws/pkg/clients"
	"github.com/crossplane/provider-aws/pkg/clients/eks"
	"github.com/crossplane/provider-aws/pkg/clients/eks/fake"
)

const (
	providerName    = "aws-creds"
	secretNamespace = "crossplane-system"
	testRegion      = "us-east-1"

	connectionSecretName = "my-little-secret"
	secretKey            = "credentials"
	credData             = "confidential!"
)

var (
	version           = "1.16"
	desiredSize int64 = 3

	errBoom = errors.New("boom")
)

type args struct {
	eks  eks.Client
	kube client.Client
	cr   *v1alpha1.NodeGroup
}

type nodeGroupModifier func(*v1alpha1.NodeGroup)

func withConditions(c ...runtimev1alpha1.Condition) nodeGroupModifier {
	return func(r *v1alpha1.NodeGroup) { r.Status.ConditionedStatus.Conditions = c }
}

func withBindingPhase(p runtimev1alpha1.BindingPhase) nodeGroupModifier {
	return func(r *v1alpha1.NodeGroup) { r.Status.SetBindingPhase(p) }
}

func withTags(tagMaps ...map[string]string) nodeGroupModifier {
	tags := map[string]string{}
	for _, tagMap := range tagMaps {
		for k, v := range tagMap {
			tags[k] = v
		}
	}
	return func(r *v1alpha1.NodeGroup) { r.Spec.ForProvider.Tags = tags }
}

func withVersion(v *string) nodeGroupModifier {
	return func(r *v1alpha1.NodeGroup) { r.Spec.ForProvider.Version = v }
}

func withStatus(s v1alpha1.NodeGroupStatusType) nodeGroupModifier {
	return func(r *v1alpha1.NodeGroup) { r.Status.AtProvider.Status = s }
}

func withScalingConfig(c *v1alpha1.NodeGroupScalingConfig) nodeGroupModifier {
	return func(r *v1alpha1.NodeGroup) { r.Spec.ForProvider.ScalingConfig = c }
}

func nodeGroup(m ...nodeGroupModifier) *v1alpha1.NodeGroup {
	cr := &v1alpha1.NodeGroup{
		Spec: v1alpha1.NodeGroupSpec{
			ResourceSpec: runtimev1alpha1.ResourceSpec{
				ProviderReference: runtimev1alpha1.Reference{Name: providerName},
			},
		},
	}
	for _, f := range m {
		f(cr)
	}
	return cr
}

var _ managed.ExternalClient = &external{}
var _ managed.ExternalConnecter = &connector{}

func TestConnect(t *testing.T) {
	secret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      connectionSecretName,
			Namespace: secretNamespace,
		},
		Data: map[string][]byte{
			secretKey: []byte(credData),
		},
	}

	providerSA := func(saVal bool) awsv1alpha3.Provider {
		return awsv1alpha3.Provider{
			Spec: awsv1alpha3.ProviderSpec{
				Region:            testRegion,
				UseServiceAccount: &saVal,
				ProviderSpec: runtimev1alpha1.ProviderSpec{
					CredentialsSecretRef: &runtimev1alpha1.SecretKeySelector{
						SecretReference: runtimev1alpha1.SecretReference{
							Namespace: secretNamespace,
							Name:      connectionSecretName,
						},
						Key: secretKey,
					},
				},
			},
		}
	}
	type args struct {
		kube        client.Client
		newClientFn func(ctx context.Context, credentials []byte, region string, auth awsclients.AuthMethod) (eks.Client, eks.STSClient, error)
		cr          *v1alpha1.NodeGroup
	}
	type want struct {
		err error
	}

	cases := map[string]struct {
		args
		want
	}{
		"Successful": {
			args: args{
				kube: &test.MockClient{
					MockGet: func(_ context.Context, key client.ObjectKey, obj runtime.Object) error {
						switch key {
						case client.ObjectKey{Name: providerName}:
							p := providerSA(false)
							p.DeepCopyInto(obj.(*awsv1alpha3.Provider))
							return nil
						case client.ObjectKey{Namespace: secretNamespace, Name: connectionSecretName}:
							secret.DeepCopyInto(obj.(*corev1.Secret))
							return nil
						}
						return errBoom
					},
				},
				newClientFn: func(_ context.Context, credentials []byte, region string, _ awsclients.AuthMethod) (_ eks.Client, _ eks.STSClient, _ error) {
					if diff := cmp.Diff(credData, string(credentials)); diff != "" {
						t.Errorf("r: -want, +got:\n%s", diff)
					}
					if diff := cmp.Diff(testRegion, region); diff != "" {
						t.Errorf("r: -want, +got:\n%s", diff)
					}
					return nil, nil, nil
				},
				cr: nodeGroup(),
			},
		},
		"SuccessfulUseServiceAccount": {
			args: args{
				kube: &test.MockClient{
					MockGet: func(_ context.Context, key client.ObjectKey, obj runtime.Object) error {
						if key == (client.ObjectKey{Name: providerName}) {
							p := providerSA(true)
							p.DeepCopyInto(obj.(*awsv1alpha3.Provider))
							return nil
						}
						return errBoom
					},
				},
				newClientFn: func(_ context.Context, credentials []byte, region string, _ awsclients.AuthMethod) (_ eks.Client, _ eks.STSClient, _ error) {
					if diff := cmp.Diff("", string(credentials)); diff != "" {
						t.Errorf("r: -want, +got:\n%s", diff)
					}
					if diff := cmp.Diff(testRegion, region); diff != "" {
						t.Errorf("r: -want, +got:\n%s", diff)
					}
					return nil, nil, nil
				},
				cr: nodeGroup(),
			},
		},
		"ProviderGetFailed": {
			args: args{
				kube: &test.MockClient{
					MockGet: func(_ context.Context, key client.ObjectKey, obj runtime.Object) error {
						return errBoom
					},
				},
				cr: nodeGroup(),
			},
			want: want{
				err: errors.Wrap(errBoom, errGetProvider),
			},
		},
		"SecretGetFailed": {
			args: args{
				kube: &test.MockClient{
					MockGet: func(_ context.Context, key client.ObjectKey, obj runtime.Object) error {
						switch key {
						case client.ObjectKey{Name: providerName}:
							p := providerSA(false)
							p.DeepCopyInto(obj.(*awsv1alpha3.Provider))
							return nil
						case client.ObjectKey{Namespace: secretNamespace, Name: connectionSecretName}:
							return errBoom
						default:
							return nil
						}
					},
				},
				cr: nodeGroup(),
			},
			want: want{
				err: errors.Wrap(errBoom, errGetProviderSecret),
			},
		},
		"SecretGetFailedNil": {
			args: args{
				kube: &test.MockClient{
					MockGet: func(_ context.Context, key client.ObjectKey, obj runtime.Object) error {
						switch key {
						case client.ObjectKey{Name: providerName}:
							p := providerSA(false)
							p.SetCredentialsSecretReference(nil)
							p.DeepCopyInto(obj.(*awsv1alpha3.Provider))
							return nil
						case client.ObjectKey{Namespace: secretNamespace, Name: connectionSecretName}:
							return errBoom
						default:
							return nil
						}
					},
				},
				cr: nodeGroup(),
			},
			want: want{
				err: errors.New(errGetProviderSecret),
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			c := &connector{kube: tc.kube, newClientFn: tc.newClientFn}
			_, err := c.Connect(context.Background(), tc.args.cr)
			if diff := cmp.Diff(tc.err, err, test.EquateErrors()); diff != "" {
				t.Errorf("r: -want, +got:\n%s", diff)
			}
		})
	}
}

func TestObserve(t *testing.T) {
	type want struct {
		cr     *v1alpha1.NodeGroup
		result managed.ExternalObservation
		err    error
	}

	cases := map[string]struct {
		args
		want
	}{
		"SuccessfulAvailable": {
			args: args{
				eks: &fake.MockClient{
					MockDescribeNodegroupRequest: func(_ *awseks.DescribeNodegroupInput) awseks.DescribeNodegroupRequest {
						return awseks.DescribeNodegroupRequest{
							Request: &aws.Request{HTTPRequest: &http.Request{}, Retryer: aws.NoOpRetryer{}, Data: &awseks.DescribeNodegroupOutput{
								Nodegroup: &awseks.Nodegroup{
									Status: awseks.NodegroupStatusActive,
								},
							}},
						}
					},
				},
				cr: nodeGroup(),
			},
			want: want{
				cr: nodeGroup(
					withConditions(runtimev1alpha1.Available()),
					withBindingPhase(runtimev1alpha1.BindingPhaseUnbound),
					withStatus(v1alpha1.NodeGroupStatusActive)),
				result: managed.ExternalObservation{
					ResourceExists:   true,
					ResourceUpToDate: true,
				},
			},
		},
		"DeletingState": {
			args: args{
				eks: &fake.MockClient{
					MockDescribeNodegroupRequest: func(_ *awseks.DescribeNodegroupInput) awseks.DescribeNodegroupRequest {
						return awseks.DescribeNodegroupRequest{
							Request: &aws.Request{HTTPRequest: &http.Request{}, Retryer: aws.NoOpRetryer{}, Data: &awseks.DescribeNodegroupOutput{
								Nodegroup: &awseks.Nodegroup{
									Status: awseks.NodegroupStatusDeleting,
								},
							}},
						}
					},
				},
				cr: nodeGroup(),
			},
			want: want{
				cr: nodeGroup(
					withConditions(runtimev1alpha1.Deleting()),
					withStatus(v1alpha1.NodeGroupStatusDeleting)),
				result: managed.ExternalObservation{
					ResourceExists:   true,
					ResourceUpToDate: true,
				},
			},
		},
		"FailedState": {
			args: args{
				eks: &fake.MockClient{
					MockDescribeNodegroupRequest: func(_ *awseks.DescribeNodegroupInput) awseks.DescribeNodegroupRequest {
						return awseks.DescribeNodegroupRequest{
							Request: &aws.Request{HTTPRequest: &http.Request{}, Retryer: aws.NoOpRetryer{}, Data: &awseks.DescribeNodegroupOutput{
								Nodegroup: &awseks.Nodegroup{
									Status: awseks.NodegroupStatusDegraded,
								},
							}},
						}
					},
				},
				cr: nodeGroup(),
			},
			want: want{
				cr: nodeGroup(
					withConditions(runtimev1alpha1.Unavailable()),
					withStatus(v1alpha1.NodeGroupStatusDegraded)),
				result: managed.ExternalObservation{
					ResourceExists:   true,
					ResourceUpToDate: true,
				},
			},
		},
		"FailedDescribeRequest": {
			args: args{
				eks: &fake.MockClient{
					MockDescribeNodegroupRequest: func(_ *awseks.DescribeNodegroupInput) awseks.DescribeNodegroupRequest {
						return awseks.DescribeNodegroupRequest{
							Request: &aws.Request{HTTPRequest: &http.Request{}, Error: errBoom},
						}
					},
				},
				cr: nodeGroup(),
			},
			want: want{
				cr:  nodeGroup(),
				err: errors.Wrap(errBoom, errDescribeFailed),
			},
		},
		"NotFound": {
			args: args{
				eks: &fake.MockClient{
					MockDescribeNodegroupRequest: func(_ *awseks.DescribeNodegroupInput) awseks.DescribeNodegroupRequest {
						return awseks.DescribeNodegroupRequest{
							Request: &aws.Request{HTTPRequest: &http.Request{}, Error: errors.New(awseks.ErrCodeResourceNotFoundException)},
						}
					},
				},
				cr: nodeGroup(),
			},
			want: want{
				cr: nodeGroup(),
			},
		},
		"LateInitSuccess": {
			args: args{
				kube: &test.MockClient{
					MockUpdate: test.NewMockUpdateFn(nil),
				},
				eks: &fake.MockClient{
					MockDescribeNodegroupRequest: func(_ *awseks.DescribeNodegroupInput) awseks.DescribeNodegroupRequest {
						return awseks.DescribeNodegroupRequest{
							Request: &aws.Request{HTTPRequest: &http.Request{}, Retryer: aws.NoOpRetryer{}, Data: &awseks.DescribeNodegroupOutput{
								Nodegroup: &awseks.Nodegroup{
									Status:  awseks.NodegroupStatusCreating,
									Version: &version,
								},
							}},
						}
					},
				},
				cr: nodeGroup(),
			},
			want: want{
				cr: nodeGroup(
					withStatus(v1alpha1.NodeGroupStatusCreating),
					withConditions(runtimev1alpha1.Creating()),
					withVersion(&version),
				),
				result: managed.ExternalObservation{
					ResourceExists:   true,
					ResourceUpToDate: true,
				},
			},
		},
		"LateInitFailedKubeUpdate": {
			args: args{
				kube: &test.MockClient{
					MockUpdate: test.NewMockUpdateFn(errBoom),
				},
				eks: &fake.MockClient{
					MockDescribeNodegroupRequest: func(_ *awseks.DescribeNodegroupInput) awseks.DescribeNodegroupRequest {
						return awseks.DescribeNodegroupRequest{
							Request: &aws.Request{HTTPRequest: &http.Request{}, Retryer: aws.NoOpRetryer{}, Data: &awseks.DescribeNodegroupOutput{
								Nodegroup: &awseks.Nodegroup{
									Status:  awseks.NodegroupStatusCreating,
									Version: &version,
								},
							}},
						}
					},
				},
				cr: nodeGroup(),
			},
			want: want{
				cr:  nodeGroup(withVersion(&version)),
				err: errors.Wrap(errBoom, errKubeUpdateFailed),
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			e := &external{kube: tc.kube, client: tc.eks}
			o, err := e.Observe(context.Background(), tc.args.cr)

			if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
				t.Errorf("r: -want, +got:\n%s", diff)
			}
			if diff := cmp.Diff(tc.want.cr, tc.args.cr, test.EquateConditions()); diff != "" {
				t.Errorf("r: -want, +got:\n%s", diff)
			}
			if diff := cmp.Diff(tc.want.result, o); diff != "" {
				t.Errorf("r: -want, +got:\n%s", diff)
			}
		})
	}
}

func TestCreate(t *testing.T) {
	type want struct {
		cr     *v1alpha1.NodeGroup
		result managed.ExternalCreation
		err    error
	}

	cases := map[string]struct {
		args
		want
	}{
		"Successful": {
			args: args{
				eks: &fake.MockClient{
					MockCreateNodegroupRequest: func(input *awseks.CreateNodegroupInput) awseks.CreateNodegroupRequest {
						return awseks.CreateNodegroupRequest{
							Request: &aws.Request{HTTPRequest: &http.Request{}, Retryer: aws.NoOpRetryer{}, Data: &awseks.CreateNodegroupOutput{}},
						}
					},
				},
				cr: nodeGroup(),
			},
			want: want{
				cr:     nodeGroup(withConditions(runtimev1alpha1.Creating())),
				result: managed.ExternalCreation{},
			},
		},
		"SuccessfulNoNeedForCreate": {
			args: args{
				cr: nodeGroup(withStatus(v1alpha1.NodeGroupStatusCreating)),
			},
			want: want{
				cr: nodeGroup(
					withStatus(v1alpha1.NodeGroupStatusCreating),
					withConditions(runtimev1alpha1.Creating())),
			},
		},
		"FailedRequest": {
			args: args{
				eks: &fake.MockClient{
					MockCreateNodegroupRequest: func(input *awseks.CreateNodegroupInput) awseks.CreateNodegroupRequest {
						return awseks.CreateNodegroupRequest{
							Request: &aws.Request{HTTPRequest: &http.Request{}, Error: errBoom},
						}
					},
				},
				cr: nodeGroup(),
			},
			want: want{
				cr:  nodeGroup(withConditions(runtimev1alpha1.Creating())),
				err: errors.Wrap(errBoom, errCreateFailed),
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			e := &external{kube: tc.kube, client: tc.eks}
			o, err := e.Create(context.Background(), tc.args.cr)

			if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
				t.Errorf("r: -want, +got:\n%s", diff)
			}
			if diff := cmp.Diff(tc.want.cr, tc.args.cr, test.EquateConditions()); diff != "" {
				t.Errorf("r: -want, +got:\n%s", diff)
			}
			if diff := cmp.Diff(tc.want.result, o); diff != "" {
				t.Errorf("r: -want, +got:\n%s", diff)
			}
		})
	}
}

func TestUpdate(t *testing.T) {
	type want struct {
		cr     *v1alpha1.NodeGroup
		result managed.ExternalUpdate
		err    error
	}

	cases := map[string]struct {
		args
		want
	}{
		"SuccessfulAddTags": {
			args: args{
				eks: &fake.MockClient{
					MockDescribeNodegroupRequest: func(input *awseks.DescribeNodegroupInput) awseks.DescribeNodegroupRequest {
						return awseks.DescribeNodegroupRequest{
							Request: &aws.Request{HTTPRequest: &http.Request{}, Retryer: aws.NoOpRetryer{}, Data: &awseks.DescribeNodegroupOutput{
								Nodegroup: &awseks.Nodegroup{},
							}},
						}
					},
					MockUpdateNodegroupConfigRequest: func(input *awseks.UpdateNodegroupConfigInput) awseks.UpdateNodegroupConfigRequest {
						return awseks.UpdateNodegroupConfigRequest{
							Request: &aws.Request{HTTPRequest: &http.Request{}, Retryer: aws.NoOpRetryer{}, Data: &awseks.UpdateNodegroupConfigOutput{}},
						}
					},
					MockTagResourceRequest: func(input *awseks.TagResourceInput) awseks.TagResourceRequest {
						return awseks.TagResourceRequest{
							Request: &aws.Request{HTTPRequest: &http.Request{}, Retryer: aws.NoOpRetryer{}, Data: &awseks.TagResourceOutput{}},
						}
					},
				},
				cr: nodeGroup(
					withTags(map[string]string{"foo": "bar"})),
			},
			want: want{
				cr: nodeGroup(
					withTags(map[string]string{"foo": "bar"})),
			},
		},
		"SuccessfulRemoveTags": {
			args: args{
				eks: &fake.MockClient{
					MockDescribeNodegroupRequest: func(input *awseks.DescribeNodegroupInput) awseks.DescribeNodegroupRequest {
						return awseks.DescribeNodegroupRequest{
							Request: &aws.Request{HTTPRequest: &http.Request{}, Retryer: aws.NoOpRetryer{}, Data: &awseks.DescribeNodegroupOutput{
								Nodegroup: &awseks.Nodegroup{},
							}},
						}
					},
					MockUpdateNodegroupConfigRequest: func(input *awseks.UpdateNodegroupConfigInput) awseks.UpdateNodegroupConfigRequest {
						return awseks.UpdateNodegroupConfigRequest{
							Request: &aws.Request{HTTPRequest: &http.Request{}, Retryer: aws.NoOpRetryer{}, Data: &awseks.UpdateNodegroupConfigOutput{}},
						}
					},
					MockUntagResourceRequest: func(input *awseks.UntagResourceInput) awseks.UntagResourceRequest {
						return awseks.UntagResourceRequest{
							Request: &aws.Request{HTTPRequest: &http.Request{}, Retryer: aws.NoOpRetryer{}, Data: &awseks.UntagResourceOutput{}},
						}
					},
				},
				cr: nodeGroup(),
			},
			want: want{
				cr: nodeGroup(),
			},
		},
		"SuccessfulUpdateVersion": {
			args: args{
				eks: &fake.MockClient{
					MockUpdateNodegroupVersionRequest: func(input *awseks.UpdateNodegroupVersionInput) awseks.UpdateNodegroupVersionRequest {
						return awseks.UpdateNodegroupVersionRequest{
							Request: &aws.Request{HTTPRequest: &http.Request{}, Retryer: aws.NoOpRetryer{}, Data: &awseks.UpdateNodegroupVersionOutput{}},
						}
					},
					MockDescribeNodegroupRequest: func(input *awseks.DescribeNodegroupInput) awseks.DescribeNodegroupRequest {
						return awseks.DescribeNodegroupRequest{
							Request: &aws.Request{HTTPRequest: &http.Request{}, Retryer: aws.NoOpRetryer{}, Data: &awseks.DescribeNodegroupOutput{
								Nodegroup: &awseks.Nodegroup{},
							}},
						}
					},
				},
				cr: nodeGroup(withVersion(&version)),
			},
			want: want{
				cr: nodeGroup(withVersion(&version)),
			},
		},
		"SuccessfulUpdateNodeGroup": {
			args: args{
				eks: &fake.MockClient{
					MockUpdateNodegroupConfigRequest: func(input *awseks.UpdateNodegroupConfigInput) awseks.UpdateNodegroupConfigRequest {
						return awseks.UpdateNodegroupConfigRequest{
							Request: &aws.Request{HTTPRequest: &http.Request{}, Retryer: aws.NoOpRetryer{}, Data: &awseks.UpdateNodegroupConfigOutput{}},
						}
					},
					MockDescribeNodegroupRequest: func(input *awseks.DescribeNodegroupInput) awseks.DescribeNodegroupRequest {
						return awseks.DescribeNodegroupRequest{
							Request: &aws.Request{HTTPRequest: &http.Request{}, Retryer: aws.NoOpRetryer{}, Data: &awseks.DescribeNodegroupOutput{
								Nodegroup: &awseks.Nodegroup{},
							}},
						}
					},
				},
				cr: nodeGroup(withScalingConfig(&v1alpha1.NodeGroupScalingConfig{DesiredSize: &desiredSize})),
			},
			want: want{
				cr: nodeGroup(withScalingConfig(&v1alpha1.NodeGroupScalingConfig{DesiredSize: &desiredSize})),
			},
		},
		"AlreadyModifying": {
			args: args{
				cr: nodeGroup(withStatus(v1alpha1.NodeGroupStatusUpdating)),
			},
			want: want{
				cr: nodeGroup(withStatus(v1alpha1.NodeGroupStatusUpdating)),
			},
		},
		"FailedDescribe": {
			args: args{
				eks: &fake.MockClient{
					MockDescribeNodegroupRequest: func(input *awseks.DescribeNodegroupInput) awseks.DescribeNodegroupRequest {
						return awseks.DescribeNodegroupRequest{
							Request: &aws.Request{HTTPRequest: &http.Request{}, Error: errBoom},
						}
					},
				},
				cr: nodeGroup(),
			},
			want: want{
				cr:  nodeGroup(),
				err: errors.Wrap(errBoom, errDescribeFailed),
			},
		},
		"FailedUpdateConfig": {
			args: args{
				eks: &fake.MockClient{
					MockUpdateNodegroupConfigRequest: func(input *awseks.UpdateNodegroupConfigInput) awseks.UpdateNodegroupConfigRequest {
						return awseks.UpdateNodegroupConfigRequest{
							Request: &aws.Request{HTTPRequest: &http.Request{}, Error: errBoom},
						}
					},
					MockDescribeNodegroupRequest: func(input *awseks.DescribeNodegroupInput) awseks.DescribeNodegroupRequest {
						return awseks.DescribeNodegroupRequest{
							Request: &aws.Request{HTTPRequest: &http.Request{}, Retryer: aws.NoOpRetryer{}, Data: &awseks.DescribeNodegroupOutput{
								Nodegroup: &awseks.Nodegroup{},
							}},
						}
					},
				},
				cr: nodeGroup(),
			},
			want: want{
				cr:  nodeGroup(),
				err: errors.Wrap(errBoom, errUpdateConfigFailed),
			},
		},
		"FailedUpdateVersion": {
			args: args{
				eks: &fake.MockClient{
					MockUpdateNodegroupVersionRequest: func(input *awseks.UpdateNodegroupVersionInput) awseks.UpdateNodegroupVersionRequest {
						return awseks.UpdateNodegroupVersionRequest{
							Request: &aws.Request{HTTPRequest: &http.Request{}, Error: errBoom},
						}
					},
					MockDescribeNodegroupRequest: func(input *awseks.DescribeNodegroupInput) awseks.DescribeNodegroupRequest {
						return awseks.DescribeNodegroupRequest{
							Request: &aws.Request{HTTPRequest: &http.Request{}, Retryer: aws.NoOpRetryer{}, Data: &awseks.DescribeNodegroupOutput{
								Nodegroup: &awseks.Nodegroup{},
							}},
						}
					},
				},
				cr: nodeGroup(withVersion(&version)),
			},
			want: want{
				cr:  nodeGroup(withVersion(&version)),
				err: errors.Wrap(errBoom, errUpdateVersionFailed),
			},
		},
		"FailedRemoveTags": {
			args: args{
				eks: &fake.MockClient{
					MockDescribeNodegroupRequest: func(input *awseks.DescribeNodegroupInput) awseks.DescribeNodegroupRequest {
						return awseks.DescribeNodegroupRequest{
							Request: &aws.Request{HTTPRequest: &http.Request{}, Retryer: aws.NoOpRetryer{}, Data: &awseks.DescribeNodegroupOutput{
								Nodegroup: &awseks.Nodegroup{
									Tags: map[string]string{"foo": "bar"},
								},
							}},
						}
					},
					MockUntagResourceRequest: func(input *awseks.UntagResourceInput) awseks.UntagResourceRequest {
						return awseks.UntagResourceRequest{
							Request: &aws.Request{HTTPRequest: &http.Request{}, Error: errBoom},
						}
					},
				},
				cr: nodeGroup(),
			},
			want: want{
				cr:  nodeGroup(),
				err: errors.Wrap(errBoom, errAddTagsFailed),
			},
		},
		"FailedAddTags": {
			args: args{
				eks: &fake.MockClient{
					MockDescribeNodegroupRequest: func(input *awseks.DescribeNodegroupInput) awseks.DescribeNodegroupRequest {
						return awseks.DescribeNodegroupRequest{
							Request: &aws.Request{HTTPRequest: &http.Request{}, Retryer: aws.NoOpRetryer{}, Data: &awseks.DescribeNodegroupOutput{
								Nodegroup: &awseks.Nodegroup{},
							}},
						}
					},
					MockTagResourceRequest: func(input *awseks.TagResourceInput) awseks.TagResourceRequest {
						return awseks.TagResourceRequest{
							Request: &aws.Request{HTTPRequest: &http.Request{}, Error: errBoom},
						}
					},
				},
				cr: nodeGroup(withTags(map[string]string{"foo": "bar"})),
			},
			want: want{
				cr:  nodeGroup(withTags(map[string]string{"foo": "bar"})),
				err: errors.Wrap(errBoom, errAddTagsFailed),
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			e := &external{kube: tc.kube, client: tc.eks}
			u, err := e.Update(context.Background(), tc.args.cr)

			if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
				t.Errorf("r: -want, +got:\n%s", diff)
			}
			if diff := cmp.Diff(tc.want.cr, tc.args.cr, test.EquateConditions()); diff != "" {
				t.Errorf("r: -want, +got:\n%s", diff)
			}
			if diff := cmp.Diff(tc.want.result, u); diff != "" {
				t.Errorf("r: -want, +got:\n%s", diff)
			}
		})
	}
}

func TestDelete(t *testing.T) {
	type want struct {
		cr  *v1alpha1.NodeGroup
		err error
	}

	cases := map[string]struct {
		args
		want
	}{
		"Successful": {
			args: args{
				eks: &fake.MockClient{
					MockDeleteNodegroupRequest: func(input *awseks.DeleteNodegroupInput) awseks.DeleteNodegroupRequest {
						return awseks.DeleteNodegroupRequest{
							Request: &aws.Request{HTTPRequest: &http.Request{}, Retryer: aws.NoOpRetryer{}, Data: &awseks.DeleteNodegroupOutput{}},
						}
					},
				},
				cr: nodeGroup(),
			},
			want: want{
				cr: nodeGroup(withConditions(runtimev1alpha1.Deleting())),
			},
		},
		"AlreadyDeleting": {
			args: args{
				cr: nodeGroup(withStatus(v1alpha1.NodeGroupStatusDeleting)),
			},
			want: want{
				cr: nodeGroup(withStatus(v1alpha1.NodeGroupStatusDeleting),
					withConditions(runtimev1alpha1.Deleting())),
			},
		},
		"AlreadyDeleted": {
			args: args{
				eks: &fake.MockClient{
					MockDeleteNodegroupRequest: func(input *awseks.DeleteNodegroupInput) awseks.DeleteNodegroupRequest {
						return awseks.DeleteNodegroupRequest{
							Request: &aws.Request{HTTPRequest: &http.Request{}, Error: errors.New(awseks.ErrCodeResourceNotFoundException)},
						}
					},
				},
				cr: nodeGroup(),
			},
			want: want{
				cr: nodeGroup(withConditions(runtimev1alpha1.Deleting())),
			},
		},
		"Failed": {
			args: args{
				eks: &fake.MockClient{
					MockDeleteNodegroupRequest: func(input *awseks.DeleteNodegroupInput) awseks.DeleteNodegroupRequest {
						return awseks.DeleteNodegroupRequest{
							Request: &aws.Request{HTTPRequest: &http.Request{}, Error: errBoom},
						}
					},
				},
				cr: nodeGroup(),
			},
			want: want{
				cr:  nodeGroup(withConditions(runtimev1alpha1.Deleting())),
				err: errors.Wrap(errBoom, errDeleteFailed),
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			e := &external{kube: tc.kube, client: tc.eks}
			err := e.Delete(context.Background(), tc.args.cr)

			if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
				t.Errorf("r: -want, +got:\n%s", diff)
			}
			if diff := cmp.Diff(tc.want.cr, tc.args.cr, test.EquateConditions()); diff != "" {
				t.Errorf("r: -want, +got:\n%s", diff)
			}
		})
	}
}

func TestInitialize(t *testing.T) {
	type want struct {
		cr  *v1alpha1.NodeGroup
		err error
	}

	cases := map[string]struct {
		args
		want
	}{
		"Successful": {
			args: args{
				cr:   nodeGroup(withTags(map[string]string{"foo": "bar"})),
				kube: &test.MockClient{MockUpdate: test.NewMockUpdateFn(nil)},
			},
			want: want{
				cr: nodeGroup(withTags(resource.GetExternalTags(nodeGroup()), (map[string]string{"foo": "bar"}))),
			},
		},
		"UpdateFailed": {
			args: args{
				cr:   nodeGroup(),
				kube: &test.MockClient{MockUpdate: test.NewMockUpdateFn(errBoom)},
			},
			want: want{
				err: errors.Wrap(errBoom, errKubeUpdateFailed),
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			e := &tagger{kube: tc.kube}
			err := e.Initialize(context.Background(), tc.args.cr)

			if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
				t.Errorf("r: -want, +got:\n%s", diff)
			}
			if diff := cmp.Diff(tc.want.cr, tc.args.cr); err == nil && diff != "" {
				t.Errorf("r: -want, +got:\n%s", diff)
			}
		})
	}
}
