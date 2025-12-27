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
      version: v1.0.0
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
          outputResponseHeaders:
            Set-Cookie: "session=; Path=/; Domain=.localhost; HttpOnly; Secure; Max-Age=0"
            X-Auth-Redirect: "true"
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
- `outputResponseHeaders`: optional map of custom response headers to set during the redirect. Useful for clearing cookies or setting custom headers.

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

When clearing cookies, ensure the `Domain` and `Path` match exactly how the cookie was originally set:

```yaml
outputResponseHeaders:
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
        outputResponseHeaders:
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
        outputResponseHeaders:
          X-Redirect-Reason: "Authentication required"
          Cache-Control: "no-cache, no-store, must-revalidate"
          Set-Cookie: "auth_token=; Path=/; Expires=Thu, 01 Jan 1970 00:00:00 GMT"
```

```