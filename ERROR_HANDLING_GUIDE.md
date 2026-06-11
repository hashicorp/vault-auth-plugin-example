# Vault Plugin Error Handling Guide

This guide demonstrates how Vault plugins return specific HTTP status codes to
clients using `logical.CodedError()`.

## Overview

Vault plugins communicate errors to clients through HTTP status codes. The
error handling mechanism works across the plugin boundary via gRPC, preserving
status codes through the entire pipeline:

```
Plugin Error → gRPC Translation → Vault Core → HTTP Response → Client
```

## Best Practices Summary

### ✅ Do

1. Always use `logical.CodedError()` for all errors
1. Be explicit about status codes
1. Return specific 4xx errors for errors internal to Vault (validation, auth, etc.)
1. Translate external system errors to 502/504 (bad gateway/timeout)
1. Prefer `%w` for error wrapping to preserve error chains for making test assertions
1. Use warnings for non-error information in successful responses

### ❌ Don't
1. Never return generic `errors.New()` - they become HTTP 500
1. Don't return both response and error - response data is lost
1. Don't pass through external system error codes unless there's good justification and consensus

### Error Code Guidelines

**General Rule:**
- **4xx errors** → Errors internal to Vault (validation, authentication, authorization, etc.)
- **502/504 errors** → Errors from external systems (translate external errors)

**Special Case:**
- Pass through error from external system **only** with good justification and consensus

**Note**: Even in pass-through cases, clearly indicate in the error message that the error originates from an external system, not Vault itself.

### Benefits of This Pattern

- Explicit HTTP status codes
- Consistent pattern across all errors
- Complete coverage of all HTTP status codes
- Clear, maintainable code

## The Pattern: `logical.CodedError()`

**Always use `logical.CodedError()` for all errors:**

```go
// Vault Client Errors (4xx)
return nil, logical.CodedError(400, "invalid input")
return nil, logical.CodedError(401, "authentication required")
return nil, logical.CodedError(403, "insufficient permissions")
return nil, logical.CodedError(404, "resource not found")
return nil, logical.CodedError(409, "resource already exists")
return nil, logical.CodedError(422, "validation failed")
return nil, logical.CodedError(429, "rate limit exceeded")

// Server Errors (5xx)
return nil, logical.CodedError(500, "internal error")
return nil, logical.CodedError(502, "external system error")
return nil, logical.CodedError(503, "service unavailable")
return nil, logical.CodedError(504, "external system timeout")
```

The Vault API will return an error response with the specified HTTP status code
and message:

```json
$ curl -s -v -H "X-Vault-Token: root" http://127.0.0.1:8200/v1/auth/example/coded-error/400
...
> GET /v1/auth/example/coded-error/400 HTTP/1.1
...
< HTTP/1.1 400 Bad Request
...

{"errors":["bad request: invalid input provided"]}
```



## Common HTTP Status Codes

| Code | Meaning | When to Use |
|------|---------|-------------|
| **4xx - Internal to Vault** | | |
| 400 | Bad Request | Invalid input validation (Vault validates) |
| 401 | Unauthorized | Authentication failed (Vault authenticates) |
| 403 | Forbidden | Authorization denied (Vault policy engine) |
| 404 | Not Found | Resource not found (in Vault storage) |
| 409 | Conflict | Resource conflict (in Vault) |
| 422 | Unprocessable Entity | Semantic validation failed (Vault logic) |
| **5xx - External Systems** | | |
| 500 | Internal Server Error | Unexpected Vault plugin error (avoid!) |
| 502 | Bad Gateway | **External system error (translate all external errors)** |
| 503 | Service Unavailable | Vault overloaded/sealed |
| 504 | Gateway Timeout | **External system timeout** |


## Error Wrapping

Error codes are preserved through wrapping:

```go
baseErr := logical.CodedError(http.StatusForbidden, "access denied")
wrappedErr := fmt.Errorf("authentication failed for user %s: %w", username, baseErr)
return nil, wrappedErr
// Still returns HTTP 403
```

Prefer `%w` for error wrapping to preserve error chains for making test assertions.

**Warning**: Vault constructs the final client API error string by evaluating
the entire error chain. Ensure that any context added via fmt.Errorf does not
contain sensitive information.

## Response + Error Anti-Pattern

**Don't do this:**

```go
// ❌ BAD: Response data is DISCARDED
return &logical.Response{
    Data: map[string]interface{}{
        "user_id": "12345",  // This is LOST
    },
}, logical.CodedError(http.StatusForbidden, "insufficient permissions")
```

**The client receives:**
- HTTP 403 status code
- Error message in response body
- **NOT** the response data

**Instead, use warnings for non-error information:**

```go
// ✅ GOOD: Success with warnings
return &logical.Response{
    Data: map[string]interface{}{
        "authenticated": true,
        "user_id":       "12345",
    },
    Warnings: []string{
        "This authentication method is deprecated",
    },
}, nil
```

## Architecture Details

### gRPC Translation

Errors are translated through protobuf when crossing the plugin boundary:

**Plugin → gRPC** (`sdk/plugin/pb/translation.go`):
```go
case logical.HTTPCodedError:
    pbErr.ErrType = ErrTypeCodedError
    pbErr.ErrCode = int64(e.(logical.HTTPCodedError).Code())
```

**gRPC → Vault Core** (`sdk/plugin/pb/translation.go`):
```go
case ErrTypeCodedError:
    err = logical.CodedError(int(e.ErrCode), e.ErrMsg)
```

### Status Code Resolution

Vault Core resolves the final status code (`sdk/logical/response_util.go`):

1. **Default status codes:**
   - No response + error = 500
   - Response + error = 400

2. **Error type overrides:**
   - `HTTPCodedError` → Uses specified code
   - Predefined errors → Uses mapped code
   - Generic errors → Uses default


## What NOT to Do

### ❌ Generic Errors
```go
// DON'T: Returns HTTP 500
return nil, errors.New("something went wrong")

// DO: Be explicit with status code
return nil, logical.CodedError(502, "external system error occurred")
```

### ❌ Response + Error
```go
// DON'T: Response data is discarded
return &logical.Response{Data: data}, logical.CodedError(403, "forbidden")

// DO: Return error only, or success with warnings
return nil, logical.CodedError(403, "forbidden")
// OR
return &logical.Response{
    Data: data,
    Warnings: []string{"deprecated endpoint"},
}, nil
```

### Common Patterns in Vault Plugins

These are common patterns you may see in existing Vault plugins. While they may
be technically valid, prefer using `logical.CodedError()` for clarity and
consistency.

#### logical.ErrorResponse() Pattern

```go
// Returns HTTP 400 (Bad Request) with error in response body
return logical.ErrorResponse("invalid input"), nil
```

Equivalent to:
```go
return nil, logical.CodedError(400, "invalid input")
```

#### nil + Error Pattern

```go
// Returns HTTP 500 (internal error) with error in response body
return nil, errors.New("internal error occurred")
```

Equivalent to:
```go
return nil, logical.CodedError(500, "internal error occurred")
```

## Testing the Examples

After building and registering the plugin, you can test different HTTP status
codes:

```bash
# Test various status codes
vault read auth/example/coded-error/400  # Bad Request
vault read auth/example/coded-error/401  # Unauthorized
vault read auth/example/coded-error/403  # Forbidden
vault read auth/example/coded-error/404  # Not Found
vault read auth/example/coded-error/500  # Internal Server Error
vault read auth/example/coded-error/502  # Bad Gateway
vault read auth/example/coded-error/504  # Gateway Timeout
```

