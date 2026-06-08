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
1. Wrap external errors with appropriate status codes (502, 504)
1. Prefer `%w` for error wrapping to preserve error chains for making test assertions
1. Use warnings for non-error information in successful responses

### ❌ Don't
1. Never return generic `errors.New()` - they become HTTP 500
1. Don't return both response and error - response data is lost

### Benefits of This Pattern

- Explicit HTTP status codes
- Consistent pattern across all errors
- Complete coverage of all HTTP status codes
- Clear, maintainable code

## The Pattern: `logical.CodedError()`

**Always use `logical.CodedError()` for all errors:**

```go
// Client Errors (4xx)
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

**Why this works:**
- Implements `logical.HTTPCodedError` interface
- Preserved through gRPC translation layer
- Explicitly sets the HTTP status code
- Works for any HTTP status code
- Consistent pattern across all errors

## Common HTTP Status Codes

| Code | Meaning | When to Use |
|------|---------|-------------|
| 400 | Bad Request | Invalid client input, validation errors |
| 401 | Unauthorized | Authentication required or failed |
| 403 | Forbidden | Authenticated but not authorized |
| 404 | Not Found | Resource doesn't exist |
| 409 | Conflict | Resource already exists |
| 429 | Too Many Requests | Rate limiting |
| 500 | Internal Server Error | Unexpected plugin error (avoid!) |
| 502 | Bad Gateway | External system error |
| 503 | Service Unavailable | System overloaded/sealed |
| 504 | Gateway Timeout | External system timeout |

## Handling External System Errors

When integrating with external systems (LDAP, databases, APIs), wrap their errors appropriately:

```go
// Example: LDAP connection error
conn, err := ldap.Dial("tcp", server)
if err != nil {
    // Network/connection errors → 502 Bad Gateway
    return nil, logical.CodedError(http.StatusBadGateway,
        fmt.Sprintf("failed to connect to LDAP server: %w", err))
}

// Example: LDAP timeout
ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
defer cancel()
result, err := conn.SearchWithContext(ctx, searchRequest)
if err != nil {
    if errors.Is(err, context.DeadlineExceeded) {
        // Timeout → 504 Gateway Timeout
        return nil, logical.CodedError(http.StatusGatewayTimeout,
            "LDAP search timed out")
    }
    // Other LDAP errors → 502 Bad Gateway
    return nil, logical.CodedError(http.StatusBadGateway,
        fmt.Sprintf("LDAP search failed: %w", err))
}

// Example: User not found in LDAP
if len(result.Entries) == 0 {
    // Not found → 404 Not Found
    return nil, logical.CodedError(http.StatusNotFound,
        "user not found in LDAP directory")
}
```

## Error Wrapping

Error codes are preserved through wrapping:

```go
baseErr := logical.CodedError(http.StatusForbidden, "access denied")
wrappedErr := fmt.Errorf("authentication failed for user %s: %w", username, baseErr)
return nil, wrappedErr
// Still returns HTTP 403
```

Prefer `%w` for error wrapping to preserve error chains for making test assertions.

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

3. **Final adjustment** (`sdk/logical/response_util.go`):
```go
// Allow HTTPCoded error passthrough to specify a code
if t, ok := err.(HTTPCodedError); ok {
    *status = t.Code()
}
```

## What NOT to Do

### ❌ Generic Errors
```go
// DON'T: Returns HTTP 500 (looks like a bug)
return nil, errors.New("something went wrong")

// DO: Be explicit with status code
return nil, logical.CodedError(500, "internal error occurred")
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

