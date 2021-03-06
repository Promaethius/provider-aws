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

package authorizer

import (
	"context"

	svcsdk "github.com/aws/aws-sdk-go/service/apigatewayv2"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/crossplane/crossplane-runtime/apis/core/v1alpha1"
	"github.com/crossplane/crossplane-runtime/pkg/event"
	"github.com/crossplane/crossplane-runtime/pkg/logging"
	"github.com/crossplane/crossplane-runtime/pkg/meta"
	"github.com/crossplane/crossplane-runtime/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/pkg/resource"

	svcapitypes "github.com/crossplane/provider-aws/apis/apigatewayv2/v1alpha1"
	aws "github.com/crossplane/provider-aws/pkg/clients"
)

// SetupAuthorizer adds a controller that reconciles Authorizer.
func SetupAuthorizer(mgr ctrl.Manager, l logging.Logger) error {
	name := managed.ControllerName(svcapitypes.AuthorizerGroupKind)
	return ctrl.NewControllerManagedBy(mgr).
		Named(name).
		For(&svcapitypes.Authorizer{}).
		Complete(managed.NewReconciler(mgr,
			resource.ManagedKind(svcapitypes.AuthorizerGroupVersionKind),
			managed.WithExternalConnecter(&connector{kube: mgr.GetClient()}),
			managed.WithLogger(l.WithValues("controller", name)),
			managed.WithRecorder(event.NewAPIRecorder(mgr.GetEventRecorderFor(name)))))
}

func (*external) preObserve(context.Context, *svcapitypes.Authorizer) error {
	return nil
}
func (*external) postObserve(_ context.Context, cr *svcapitypes.Authorizer, _ *svcsdk.GetAuthorizersOutput, obs managed.ExternalObservation, err error) (managed.ExternalObservation, error) {
	if err != nil {
		return managed.ExternalObservation{}, err
	}
	cr.SetConditions(v1alpha1.Available())
	return obs, nil
}

func (*external) filterList(cr *svcapitypes.Authorizer, list *svcsdk.GetAuthorizersOutput) *svcsdk.GetAuthorizersOutput {
	res := &svcsdk.GetAuthorizersOutput{}
	for _, authorizer := range list.Items {
		if meta.GetExternalName(cr) == aws.StringValue(authorizer.Name) {
			res.Items = append(res.Items, authorizer)
		}
	}
	return res
}

func (*external) preCreate(context.Context, *svcapitypes.Authorizer) error {
	return nil
}

func (*external) postCreate(_ context.Context, _ *svcapitypes.Authorizer, _ *svcsdk.CreateAuthorizerOutput, cre managed.ExternalCreation, err error) (managed.ExternalCreation, error) {
	return cre, err
}

func (*external) preUpdate(context.Context, *svcapitypes.Authorizer) error {
	return nil
}

func (*external) postUpdate(_ context.Context, _ *svcapitypes.Authorizer, upd managed.ExternalUpdate, err error) (managed.ExternalUpdate, error) {
	return upd, err
}
func lateInitialize(*svcapitypes.AuthorizerParameters, *svcsdk.GetAuthorizersOutput) error {
	return nil
}

func preGenerateGetAuthorizersInput(_ *svcapitypes.Authorizer, obj *svcsdk.GetAuthorizersInput) *svcsdk.GetAuthorizersInput {
	return obj
}

func postGenerateGetAuthorizersInput(cr *svcapitypes.Authorizer, obj *svcsdk.GetAuthorizersInput) *svcsdk.GetAuthorizersInput {
	obj.ApiId = cr.Spec.ForProvider.APIID
	return obj
}

func preGenerateCreateAuthorizerInput(_ *svcapitypes.Authorizer, obj *svcsdk.CreateAuthorizerInput) *svcsdk.CreateAuthorizerInput {
	return obj
}

func postGenerateCreateAuthorizerInput(cr *svcapitypes.Authorizer, obj *svcsdk.CreateAuthorizerInput) *svcsdk.CreateAuthorizerInput {
	obj.ApiId = cr.Spec.ForProvider.APIID
	obj.Name = aws.String(meta.GetExternalName(cr))
	return obj
}

func preGenerateDeleteAuthorizerInput(_ *svcapitypes.Authorizer, obj *svcsdk.DeleteAuthorizerInput) *svcsdk.DeleteAuthorizerInput {
	return obj
}

func postGenerateDeleteAuthorizerInput(cr *svcapitypes.Authorizer, obj *svcsdk.DeleteAuthorizerInput) *svcsdk.DeleteAuthorizerInput {
	obj.ApiId = cr.Spec.ForProvider.APIID
	obj.AuthorizerId = cr.Status.AtProvider.AuthorizerID
	return obj
}
