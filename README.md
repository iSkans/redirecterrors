# Redirect On Errors

A [Traefik](https://traefik.io) middleware plugin to redirect to another URL on specific HTTP statuses.

Similar to the built-in `Errors` middleware, but this generates a HTTP 302 redirect, instead of an internal proxy action.

It was created to make `ForwardAuth` easier to use.

### Example Configuration

Example configuration with `ForwardAuth`:

```yaml
# Static configuration

experimental:
  plugins:
    redirectErrors:
      moduleName: github.com/iskans/redirecterrors
      version: v1.1.0
```

```yaml
# Dynamic configuration

http:
  routers:
    secured-router:
      rule: host(`secured.localhost`)
      service: service-secured
      middlewares:
        - auth-redirect
    auth-router:
      rule: host(`auth.localhost`)
      service: service-auth
      middlewares:
        - my-plugin

  services:
    service-secured:
      loadBalancer:
        servers:
          - url: http://localhost:5001/
    service-auth:
      loadBalancer:
        servers:
          - url: http://localhost:5002/

  middlewares:
    auth-redirect-error:
      plugin:
        redirectErrors:
          status:
            - "401"
          target: "http://auth.localhost/oauth2/sign_in?rd={url}"
          outputStatus: 302
    auth-check:
      forwardAuth:
        address: "http://localhost:5002/oauth2/auth"
        trustForwardHeader: true
    auth-redirect:
      chain:
        - auth-redirect-error
        - auth-check
```

### Configuration Options

- `status`: list of statuses / status ranges (eg `401-403`). See the [Error middleware's description](https://doc.traefik.io/traefik/middlewares/http/errorpages/#status) for details.
- `target`: redirect target URL. `{status}` will be replaced with the original HTTP status code, and `{url}` will be replaced with the url-safe version of the original, full URL.
- `outputStatus`: HTTP code for the redirect. Default is `302`.
- `outputAddHeaders`: optional map of custom response headers to set during the redirect. Useful for clearing cookies or setting custom headers.
- `outputRemoveHeaders`: optional list of regex patterns. Headers matching any pattern will be removed from the redirect response. Useful for stripping sensitive headers from forwardAuth responses (e.g., `^Authentik-Proxy-.+$`).
- `outputAddCookies`: optional list of Set-Cookie header values to add during the redirect (e.g., `session=123; Path=/; HttpOnly; Secure`).
- `outputRemoveCookies`: optional list of regex patterns. Request cookies matching any pattern will be deleted via Set-Cookie with `Max-Age=0` (e.g., `^authentik_proxy_.+$`).

### Best Practices

#### Middleware Order with ForwardAuth

**Always place this middleware BEFORE `forwardAuth`** in a middleware chain. The plugin needs to intercept the error response from `forwardAuth`, so it must come first:

```yaml
middlewares:
  auth-redirect:
    chain:
      - auth-redirect-error    # This plugin FIRST
      - auth-check             # Then forwardAuth
```

#### Choosing the Right Redirect Status

- **302 (Found)**: Default. Use for temporary redirects. Most browsers will cache the redirect but may re-send POST data as GET.
- **307 (Temporary Redirect)**: Preserves the HTTP method (POST stays POST). Use when the original request method matters.
- **303 (See Other)**: Converts POST to GET. Use after form submissions to prevent duplicate submissions on refresh.
- **301 (Moved Permanently)**: Use only when the redirect is truly permanent. Browsers cache this aggressively.

#### Clearing Cookies on Authentication Failure

You can clear cookies using either `outputAddHeaders` with Set-Cookie, or use `outputRemoveCookies` to automatically remove matching request cookies.

When clearing cookies manually via headers, ensure the `Domain` and `Path` match exactly how the cookie was originally set:

```yaml
outputAddHeaders:
  Set-Cookie: "session=; Path=/; Domain=.example.com; HttpOnly; Secure; Max-Age=0"
```

Key parameters for cookie deletion:
- `Max-Age=0`: Immediately expires the cookie
- `Expires=Thu, 01 Jan 1970 00:00:00 GMT`: Alternative expiration method
- `Domain=`: Must match the original cookie's domain (include the leading dot for wildcard domains)
- `Path=/`: Should cover all paths where the cookie was set

#### X-Forwarded Headers

This plugin reconstructs the original URL using `X-Forwarded-Proto` and `X-Forwarded-Host` headers. Ensure your Traefik configuration has `trustForwardHeader: true` when using `forwardAuth`, or these headers may be missing.

#### Security Considerations

- Always validate the redirect target URL on your auth service
- Use HTTPS for your `target` URL in production
- Consider setting `Cache-Control: no-cache` headers to prevent caching of redirect responses
- Be aware that the `{url}` template variable contains user input and should be validated by the receiving service

### Examples with Custom Headers

Clear session cookies on authentication errors:

```yaml
middlewares:
  auth-redirect-error:
    plugin:
      redirectErrors:
        status:
          - "401"
          - "403"
        target: "http://auth.localhost/oauth2/sign_in?rd={url}"
        outputStatus: 302
        outputAddHeaders:
          Set-Cookie: "session=; Path=/; Domain=.localhost; HttpOnly; Secure; Max-Age=0"
          X-Auth-Required: "true"
```

Set multiple custom headers:

```yaml
middlewares:
  redirect-with-headers:
    plugin:
      redirectErrors:
        status:
          - "401-403"
        target: "http://login.example.com/?return={url}"
        outputStatus: 307
        outputAddHeaders:
          X-Redirect-Reason: "Authentication required"
          Cache-Control: "no-cache, no-store, must-revalidate"
          Set-Cookie: "auth_token=; Path=/; Expires=Thu, 01 Jan 1970 00:00:00 GMT"
```

### Remove Headers with Regex Patterns

Remove sensitive headers added by forwardAuth (e.g., Authentik headers) using regex:

```yaml
middlewares:
  auth-redirect-error:
    plugin:
      redirectErrors:
        status:
          - "401"
        target: "https://login.example.com/?return={url}"
        outputStatus: 302
        outputRemoveHeaders:
          - "^[^_]+_proxy_.+$"
          - "^[^_]+_session_.+$"
```

**Note:** HTTP header names are canonicalized by Go (e.g., `authentik_proxy_user` becomes `Authentik-Proxy-User`). Use hyphens and title casing in your regex patterns.

The header removal process:
1. First copies all headers from the upstream response
2. Sets the `Location` header for redirect
3. Adds any headers from `outputAddHeaders`
4. Finally removes headers matching `outputRemoveHeaders` patterns

This order ensures `outputAddHeaders` can override upstream headers, and `outputRemoveHeaders` can clean up both upstream and added headers.

### Add and Remove Cookies

Add cookies during redirect and remove specific cookies from the request:

```yaml
middlewares:
  auth-redirect-error:
    plugin:
      redirectErrors:
        status:
          - "401"
        target: "https://login.example.com/?return={url}"
        outputStatus: 302
        outputAddCookies:
          - "session=new123; Path=/; Domain=.example.com; HttpOnly; Secure"
          - "preference=dark; Path=/; Domain=.example.com; HttpOnly; Secure"
        outputRemoveCookies:
          - "^[^_]+_proxy_.+$"
          - "^[^_]+_session_.+$"
```

**How it works:**
- `outputAddCookies`: Adds Set-Cookie headers with the specified cookie strings
- `outputRemoveCookies`: Scans request cookies and matches them against regex patterns. For each match, adds a deletion Set-Cookie header: `cookie_name=; Path=/; Max-Age=0; HttpOnly; Secure`
- Each cookie is only removed once, even if it matches multiple regex patterns

**Use case:** When redirecting to a login page due to authentication failure, you may want to:
1. Add a new session cookie or tracking cookie
2. Remove sensitive auth cookies from the request (like Authentik proxy cookies)

### Complete Example

A complete example with all features enabled:

```yaml
middlewares:
  auth-redirect-error:
    plugin:
      redirectErrors:
        status:
          - "401-403"
        target: "https://login.example.com/?return={url}"
        outputStatus: 303
        outputAddHeaders:
          X-Custom-Header: "custom-value"
          Cache-Control: "no-cache, no-store, must-revalidate"
        outputRemoveHeaders:
          - "^X-Auth-.+$"
          - "^Sec-.+$"
        outputAddCookies:
          - "session=123; Path=/; Domain=.example.com; HttpOnly; Secure"
          - "mycookie=yes; Path=/; Domain=.example.com; HttpOnly; Secure"
        outputRemoveCookies:
          - "^[^_]+_proxy_.+$"
          - "^[^_]+_session_.+$"
    forwardAuth:
      address: "http://auth-service/auth"
      trustForwardHeader: true
```

### Processing Order

The middleware processes responses in this order:
1. Copies all headers from upstream response
2. Sets `Location` header for redirect
3. Adds headers from `outputAddHeaders`
4. Removes headers matching `outputRemoveHeaders` patterns
5. Adds cookies from `outputAddCookies`
6. Removes cookies matching `outputRemoveCookies` patterns
6. Sends redirect response with configured status code

This ensures:
- `outputAddHeaders` can override upstream headers
- `outputAddCookies` adds new cookies
- `outputRemoveHeaders` can clean up both upstream and added headers
- `outputRemoveCookies` removes matching request cookies via deletion Set-Cookie headers