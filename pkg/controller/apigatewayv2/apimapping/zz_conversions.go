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

// Code generated by ack-generate. DO NOT EDIT.

package apimapping

import (
	"github.com/aws/aws-sdk-go/aws/awserr"
	svcsdk "github.com/aws/aws-sdk-go/service/apigatewayv2"

	svcapitypes "github.com/crossplane/provider-aws/apis/apigatewayv2/v1alpha1"
)

// NOTE(muvaf): We return pointers in case the function needs to start with an
// empty object, hence need to return a new pointer.
// TODO(muvaf): We can generate one-time boilerplate for these hooks but currently
// ACK doesn't support not generating if file exists.
// GenerateGetApiMappingsInput returns input for read
// operation.
func GenerateGetApiMappingsInput(cr *svcapitypes.APIMapping) *svcsdk.GetApiMappingsInput {
	res := preGenerateGetApiMappingsInput(cr, &svcsdk.GetApiMappingsInput{})

	return postGenerateGetApiMappingsInput(cr, res)
}

// GenerateAPIMapping returns the current state in the form of *svcapitypes.APIMapping.
func GenerateAPIMapping(resp *svcsdk.GetApiMappingsOutput) *svcapitypes.APIMapping {
	cr := &svcapitypes.APIMapping{}

	found := false
	for _, elem := range resp.Items {
		if elem.ApiId != nil {
			cr.Status.AtProvider.APIID = elem.ApiId
		}
		if elem.ApiMappingId != nil {
			cr.Status.AtProvider.APIMappingID = elem.ApiMappingId
		}
		if elem.ApiMappingKey != nil {
			cr.Spec.ForProvider.APIMappingKey = elem.ApiMappingKey
		}
		if elem.Stage != nil {
			cr.Status.AtProvider.Stage = elem.Stage
		}
		found = true
		break
	}
	if !found {
		return cr
	}

	return cr
}

// GenerateCreateApiMappingInput returns a create input.
func GenerateCreateApiMappingInput(cr *svcapitypes.APIMapping) *svcsdk.CreateApiMappingInput {
	res := preGenerateCreateApiMappingInput(cr, &svcsdk.CreateApiMappingInput{})

	if cr.Spec.ForProvider.APIMappingKey != nil {
		res.SetApiMappingKey(*cr.Spec.ForProvider.APIMappingKey)
	}

	return postGenerateCreateApiMappingInput(cr, res)
}

// GenerateDeleteApiMappingInput returns a deletion input.
func GenerateDeleteApiMappingInput(cr *svcapitypes.APIMapping) *svcsdk.DeleteApiMappingInput {
	res := preGenerateDeleteApiMappingInput(cr, &svcsdk.DeleteApiMappingInput{})

	if cr.Status.AtProvider.APIMappingID != nil {
		res.SetApiMappingId(*cr.Status.AtProvider.APIMappingID)
	}

	return postGenerateDeleteApiMappingInput(cr, res)
}

// IsNotFound returns whether the given error is of type NotFound or not.
func IsNotFound(err error) bool {
	awsErr, ok := err.(awserr.Error)
	return ok && awsErr.Code() == "NotFoundException"
}
