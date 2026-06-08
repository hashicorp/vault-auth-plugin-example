// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package example

import (
	"context"
	"fmt"
	"net/http"

	"github.com/hashicorp/vault/sdk/framework"
	"github.com/hashicorp/vault/sdk/logical"
)

// PathErrorExamples demonstrates different ways to return errors from Vault plugin handlers
// and the HTTP status codes that result from each approach.
// Note: Pass nil for b if you don't need backend-specific functionality
func PathErrorExamples(b *framework.Backend) *framework.Path {
	return &framework.Path{
		Pattern: "coded-error/" + framework.GenericNameRegex("status_code"),

		DisplayAttrs: &framework.DisplayAttributes{
			OperationPrefix: "example",
			OperationVerb:   "coded-error",
		},

		Fields: map[string]*framework.FieldSchema{
			"status_code": {
				Type:        framework.TypeString,
				Description: "HTTP status code to demonstrate (e.g., 400, 403, 404, 500)",
				Required:    true,
			},
		},

		Operations: map[logical.Operation]framework.OperationHandler{
			logical.ReadOperation: &framework.PathOperation{
				Callback: pathErrorExamplesRead,
				Summary:  "Demonstrates different error handling patterns and their HTTP status codes",
			},
		},

		HelpSynopsis:    "Error handling examples for Vault plugin development",
		HelpDescription: "This endpoint demonstrates various error handling patterns and the HTTP status codes they produce.",
	}
}

func pathErrorExamplesRead(ctx context.Context, req *logical.Request, d *framework.FieldData) (*logical.Response, error) {
	statusCode := d.Get("status_code").(string)

	switch statusCode {
	case "400":
		return nil, logical.CodedError(http.StatusBadRequest, "bad request: invalid input provided")

	case "401":
		return nil, logical.CodedError(http.StatusUnauthorized, "authentication required or failed")

	case "403":
		return nil, logical.CodedError(http.StatusForbidden, "insufficient permissions to access this resource")

	case "404":
		return nil, logical.CodedError(http.StatusNotFound, "requested resource does not exist")

	case "405":
		return nil, logical.CodedError(http.StatusMethodNotAllowed, "operation not supported")

	case "409":
		return nil, logical.CodedError(http.StatusConflict, "resource already exists")

	case "422":
		return nil, logical.CodedError(http.StatusUnprocessableEntity, "request validation failed")

	case "429":
		return nil, logical.CodedError(http.StatusTooManyRequests, "rate limit exceeded")

	case "500":
		return nil, logical.CodedError(http.StatusInternalServerError, "internal server error")

	case "502":
		return nil, logical.CodedError(http.StatusBadGateway, "external system error")

	case "503":
		return nil, logical.CodedError(http.StatusServiceUnavailable, "service temporarily unavailable")

	case "504":
		return nil, logical.CodedError(http.StatusGatewayTimeout, "external system timeout")

	default:
		// Unknown status code
		return nil, logical.CodedError(http.StatusBadRequest,
			fmt.Sprintf("unknown status code '%s'. Valid codes: 400, 401, 403, 404, 405, 409, 422, 429, 500, 502, 503, 504", statusCode))
	}
}

// Made with Bob
