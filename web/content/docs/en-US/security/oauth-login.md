---
title: Third-party Login
order: 7
category: Security
description: Configure Microsoft, Google, GitLab, Cloudflare, Stack Exchange, and custom OAuth providers
---

# Third-party Login

## Configure providers

In administrator settings, open **Third-party login** to configure GitHub and other providers. GitHub is the built-in
entry with the fixed ID `github`; enable or disable it there. Add up to 32 other clients with unique lowercase IDs of up
to 32 characters, starting with a letter and using letters, digits, underscores, or hyphens. Saved IDs identify existing
bindings and cannot be edited. Each account supports one GitHub binding and up to 32 other provider bindings.

GitHub retains `server.github_oauth` and the callback `/api/auth/github/callback` for compatibility. Existing settings
and bindings appear automatically in the unified interface. Its client ID is limited to 128 bytes and secret to 512
bytes; a blank secret is retained only for the same client ID. GitHub user and organization authorization, verified
email, and manual avatar synchronization remain available.

The remaining provider configuration and protocol details below describe the additional OAuth clients; GitHub retains
its existing authorization flow and compatibility callback.

Register a web application with the provider and allow the exact callback
`https://renop.example/api/auth/oauth/<provider-id>/callback`. OAuth endpoints and callbacks require HTTPS, except HTTP
loopback addresses used for development. New authorizations use saved settings immediately; changes invalidate
authorizations already in progress.

| Preset          | Application setup and options                                                                                                                                                                                                                                                                                                                                                                                 | Email handling                                                                                                                               |
|-----------------|---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|----------------------------------------------------------------------------------------------------------------------------------------------|
| `microsoft`     | [Microsoft identity platform](https://learn.microsoft.com/en-us/entra/identity-platform/v2-protocols-oidc); `tenant` is `common`, `organizations`, `consumers`, or an Entra tenant UUID. Match the application's supported account types. Default scopes: `openid profile email`.                                                                                                                             | [UserInfo](https://learn.microsoft.com/en-us/entra/identity-platform/userinfo) does not assert email verification; a RenoP code is required. |
| `google`        | [Google OpenID Connect](https://developers.google.com/identity/openid-connect/openid-connect); default scopes: `openid profile email`.                                                                                                                                                                                                                                                                        | Accepts an address only as verified when `email_verified` is boolean `true`.                                                                 |
| `gitlab`        | [GitLab OpenID Connect](https://docs.gitlab.com/integration/openid_connect_provider/); `base_url` defaults to `https://gitlab.com` and may select a self-hosted instance. Default scopes: `openid profile email`.                                                                                                                                                                                             | Requires a code if a verified contact email is not returned.                                                                                 |
| `cloudflare`    | [Create an OAuth client](https://developers.cloudflare.com/fundamentals/oauth/create-an-oauth-client/) and [integrate it](https://developers.cloudflare.com/fundamentals/oauth/integrate-with-cloudflare/). The `/oauth2/userinfo` endpoint supplies the stable subject (`sub`). Private clients are restricted to the Cloudflare account's members; public clients require Cloudflare's domain verification. | Automatically retrieves verified email if `user-details.read` is granted; otherwise a RenoP code is required.                                |
| `stackexchange` | Register a [Stack Apps application](https://stackapps.com/help/api-authentication), set its client credentials and API key, and choose `site` (default `stackoverflow`). RenoP follows the [authorization-code flow](https://api.stackexchange.com/docs/authentication) with PKCE and uses the network `account_id`.                                                                                          | No contact email is supplied; a RenoP code is required.                                                                                      |
| `custom`        | Configure authorization, token, and user-info URLs plus JSON field paths. Add issuer and JWKS settings for OpenID Connect.                                                                                                                                                                                                                                                                                    | A mapped email is trusted only with a mapped boolean `true` verification field; otherwise a code is required.                                |

Microsoft supports personal accounts and Entra ID accounts according to the tenant and application registration.
Cloudflare and Stack Exchange do not require a fabricated email mapping. If their users need to register,
enable [email delivery](../configuration/mail.md) first.

## Provider setup guides

### GitHub

Navigate to GitHub **Settings** -> **Developer settings** -> **OAuth Apps** and select **New OAuth App** (or open
Developer settings under your organization).

Set the application parameters:

- **Application name**: The display name for your app (e.g. `RenoP`).
- **Homepage URL**: The root URL of your RenoP instance (e.g. `https://renop.example`).
- **Authorization callback URL**: The dedicated GitHub callback URL `https://renop.example/api/auth/github/callback`.
  Notice that GitHub uses this fixed route rather than a dynamic provider subpath.

Register the application, generate a **Client Secret**, and record both the **Client ID** and **Client Secret**. In
RenoP administrator settings under **Third-party login**, open the built-in **GitHub** entry, supply the credentials,
and enable the provider.

### Microsoft Entra ID

Sign in to the Microsoft Entra admin center or Azure portal. Go to **Identity** -> **Applications** -> **App
registrations**, and click **New registration**.

Configure the registration:

- **Name**: Application name (e.g. `RenoP`).
- **Supported account types**:
    - For personal Microsoft accounts and any organizational directory: select "Accounts in any organizational directory
      and personal Microsoft accounts", and set `tenant: common` in RenoP.
    - For work and school accounts across organizations: select "Accounts in any organizational directory", and set
      `tenant: organizations` in RenoP.
    - For personal accounts only: select "Personal Microsoft accounts only", and set `tenant: consumers` in RenoP.
    - For your single tenant only: select "Accounts in this organizational directory only", and set `tenant` to your
      Directory (tenant) ID GUID.
- **Redirect URI**: Select platform "Web" and input `https://renop.example/api/auth/oauth/microsoft/callback`.

Under **Certificates & secrets**, add a new client secret and copy its **Value** immediately. Verify under **API
permissions** that delegated `User.Read` permission is present with scopes `openid`, `profile`, and `email`. In RenoP,
add a provider of type `microsoft`, and enter the Client ID, Client Secret, and Tenant.

### Google Cloud

Open the Google Cloud Console and navigate to **APIs & Services** -> **Credentials**.

If you have not yet configured the **OAuth consent screen**:

- Choose User Type **External** (or Internal for Google Workspace users), provide application details, and add the
  scopes `openid`, `.../auth/userinfo.email`, and `.../auth/userinfo.profile`.

Under **Credentials**, click **Create Credentials** -> **OAuth client ID**:

- **Application type**: Web application.
- **Authorized redirect URIs**: Add `https://renop.example/api/auth/oauth/google/callback`.

Save and copy the **Client ID** and **Client Secret**. In RenoP, add a provider with type `google`, and input the Client
ID and Client Secret (default scopes: `openid profile email`).

### GitLab

In GitLab.com or your self-hosted GitLab instance, create an OAuth application:

- For instance-wide access: **Admin Area** -> **Applications**.
- For your user profile: **User Settings** -> **Applications**.
- For a group: Group **Settings** -> **Applications**.

Set the parameters:

- **Name**: Application name.
- **Redirect URI**: `https://renop.example/api/auth/oauth/gitlab/callback`.
- **Confidential**: Keep enabled.
- **Scopes**: Check `openid`, `profile`, `email`, and `read_user`.

Save and record the **Application ID** (Client ID) and **Secret**. In RenoP, add a provider with type `gitlab`. If
connecting to a self-hosted instance, set `base_url` to your instance domain (e.g. `https://gitlab.example.com`).

### Cloudflare

In the Cloudflare dashboard, navigate to **Manage Account** -> **OAuth clients**.

Click **Create client** and configure the application and protocol settings:

- **Client name**: Enter a name for the client (e.g. `RenoP`).
- **Response type**: Supports Token, ID Token, and Code (multi-select). **You must select `Code`** (RenoP uses
  authorization code flow).
- **Grant type**: Supports Authorization Code and Refresh Token (multi-select). **You must select
  `Authorization Code`**.
- **Redirect URLs**: Enter the full RenoP callback URL, such as
  `https://renop.example/api/auth/oauth/cloudflare/callback` (replace `cloudflare` with your custom provider ID if
  applicable).
- **Client URL** (optional): RenoP instance homepage (e.g. `https://renop.example`). Required if you plan to promote the
  client to a Public client later (requiring domain verification via DNS TXT record); optional for Private clients used
  exclusively within your Cloudflare account.
- **Token authentication method** (select one of three, matching RenoP configuration):
    - **Client Secret POST** (default and recommended): Client sends the secret in the token exchange request body. In
      RenoP, the token authentication method defaults to `client_secret_post`.
    - **Client Secret Basic**: Client sends the secret via the HTTP Basic Authorization header. If selected in
      Cloudflare, set RenoP's token authentication method to `client_secret_basic`. RenoP also includes automatic
      negotiation if HTTP 401 is returned.
    - **PKCE**: Public client mode without client secret. Set RenoP's token authentication method to `none` (no client
      secret required).
- **Post-logout redirect URLs** (optional): Post-logout destination, such as `https://renop.example`.
- **Allowed CORS origins** (optional): Allowed cross-origin origins; can be left blank as token exchange is performed
  server-to-server by RenoP.

Click **Continue** and configure permission Scopes:

- Select the API permissions required for the client (at least one permission such as `User Details: Read`).
- **Scopes Configuration**: In RenoP's provider settings, leave the Scopes field empty (default), or enter the scopes
  granted in Cloudflare (supports space or comma-separated lists, e.g.
  `memberships.read, user-details.read, offline_access, openid`).
- **Email & Profile Synchronization**: If `user-details.read` is granted, RenoP automatically calls Cloudflare's API
  (`/client/v4/user`) during sign-in and registration to retrieve the verified account email and username, skipping
  manual email verification. If not granted, new users sign in with the subject (`sub`) provided by `/oauth2/userinfo`
  and verify their email via a RenoP email code.

Save the client and copy the **Client ID**, as well as the **Client Secret** displayed in the popup (shown only once;
note that this must be the OAuth Client Secret, not a Cloudflare API Token or Global API Key). In RenoP, add a provider
with type `cloudflare`, inputting the Client ID, Client Secret (if applicable), and matching token authentication method
(defaults to `client_secret_post`). Leading and trailing whitespace is automatically trimmed.

### Stack Exchange

Register an application on Stack Apps at `https://stackapps.com/apps/oauth/register`.

Enter the details:

- **Application Name**: Name of your application.
- **OAuth Domain**: The host domain of RenoP without scheme or port (e.g. `renop.example`).
- **Enable Client Side Flow**: Leave disabled.

Submit the form and record the **Client Id**, **Client Secret**, and the **Key** (the API key required to authenticate
API requests and quotas). In RenoP, add a provider with type `stackexchange`, providing the Client ID, Client Secret,
and API Key. The `site` option defaults to `stackoverflow`.

### Custom providers

For OpenID Connect or OAuth 2.0 servers such as Keycloak, Authentik, Authelia, Casdoor, or Dex, select provider type
`custom`.

Configure the endpoints:

- **Authorize URL**: Authorization redirect endpoint.
- **Token URL**: Token exchange endpoint.
- **UserInfo URL**: User profile information endpoint.
- **Issuer** and **JWKS URL**: Required for OIDC signature verification.
- **Scopes**: Space-delimited scopes, including `openid profile email` for OIDC.
- **Token Auth**: Choose `client_secret_post` or `client_secret_basic` as required by your provider.

Configure field mappings:

- `subject`: Path to the unique stable identifier (e.g. `sub` or `id`).
- `username` and `name`: Paths to user handles and display names.
- `email` and `email_verified`: Paths to the email address and its boolean verification flag.

## Configuration and credentials

Additional providers are stored in `server.oauth_providers`. The following example keeps clients disabled until their
credentials
are replaced:

```yaml
server:
  oauth_providers:
    - id: microsoft
      type: microsoft
      name: Microsoft
      enabled: false
      client_id: ""
      client_secret: ""
      callback_url: https://renop.example/api/auth/oauth/microsoft/callback
      tenant: common
    - id: example
      type: custom
      name: Example Identity
      enabled: false
      client_id: ""
      client_secret: ""
      callback_url: https://renop.example/api/auth/oauth/example/callback
      authorize_url: https://identity.example/authorize
      token_url: https://identity.example/token
      userinfo_url: https://identity.example/userinfo
      scopes: ""
      token_auth: client_secret_post
      disable_pkce: false
      claims:
        subject: user.id
        username: user.username
        name: user.name
        email: user.email
        email_verified: user.email_verified
        avatar: user.picture
```

`name` allows 80 UTF-8 bytes; `client_id` allows 512 bytes; secrets and API keys allow 4,096 bytes. `scopes` is a
space-separated string of at most 1,024 bytes. Endpoint URLs allow 2,048 bytes. Built-in presets supply their official
endpoints and claim mappings; use `custom` for other layouts.

Custom `claims` select scalar values using dotted JSON paths and numeric array indexes, for example `user.id` or
`items.0.id`. Paths allow 128 characters. `subject` must be a stable provider identifier; names and emails must not
serve as account identity. Numeric identifiers retain their precision. Optional profile fields may be omitted. A string
such as `"true"` does not verify an email.

Custom `token_auth` accepts `client_secret_post`, `client_secret_basic`, or `none`. PKCE S256 is enabled by default.
`disable_pkce: true` is allowed only for custom providers with a client secret. For OpenID Connect, set both `issuer`
and `jwks_url` and include `openid` in `scopes`; RenoP then requires and verifies an ID token. OAuth-only custom clients
use the authenticated user-info response.

OAuth-only custom clients include the subject-field path in their identity authority. Changing that field requires
reconnecting the account binding. If you enabled a custom OAuth-only client from commit `26a1c1a`, reconnect it after
updating to this isolation rule. OIDC clients continue to use the verified ID-token subject. Cloudflare exposes only
`sub`, so email and photo import actions are unavailable; registration uses manual account details and a RenoP email
code.

Settings reads expose `client_secret_configured` and `api_key_configured`, with secret values blank. A blank write
preserves a saved credential only for the same ID, provider type, client ID, and token endpoint. `clear_client_secret`
and `clear_api_key` explicitly remove them. Changes to a client or endpoint require re-entering credentials. Removing a
provider disables future authorizations but retains account bindings so users can disconnect them.

## Registration and account controls

An unbound provider login starts [account registration](./registration.md). No account or login session exists until the
user confirms and sets a password. The ten-minute confirmation period, provider cooldown, IP allowance, recipient
policy, local username/nickname limits, and avatar quotas apply to all providers. Existing accounts are never selected
or merged by a matching email address.

The pending response supplies `provider`, `provider_name`, `email_required`, `mail_enabled`, and `avatar_available`.
When `email_required` is true, the user must enter and verify an address even if the provider returned an unverified
suggestion. Send `provider` together with `email` to the registration code endpoint. Keep the same provider ID in the
final confirmation and include the received `code`. Code replacement does not extend the original confirmation deadline
or reset failed attempts. If mail is unavailable, a provider that requires email verification cannot complete
registration.

If the provider returns a verified address, it is fixed during confirmation and no code is needed. Importing the
username, nickname, and avatar is optional; unavailable or oversized photos do not cancel registration. Names must still
satisfy instance limits, and duplicate usernames require manual replacement.

In your profile editor, connect or refresh a provider, import its photo, use its latest verified email, or disconnect
it. Email verification requires a fresh provider authorization and does not change the login binding. Providers without
a verified-email claim do not show that action. Photo import requires the selected provider identity to be linked to the
current account. The server preserves another primary login method before
disconnecting. [Two-step verification](./two-step-verification.md), bans, and permanent account closure apply to
provider login.

## API

| Method | Path                                 | Contract                                                                                 |
|--------|--------------------------------------|------------------------------------------------------------------------------------------|
| GET    | `/api/auth/oauth/providers`          | Public list of configured provider IDs and names                                         |
| GET    | `/api/auth/oauth/:provider/start`    | `intent=login`, `register`, `link`, `email`, or `avatar`; optional local `return_to`     |
| GET    | `/api/auth/oauth/:provider/callback` | Single-use authorization code and browser-bound state                                    |
| GET    | `/api/auth/profile/oauth`            | Private connection states, display login, authorization timestamp, and permitted actions |
| DELETE | `/api/auth/profile/oauth/:provider`  | Disconnect; `204`, or `409` with `oauth_last_login_method`                               |
| GET    | `/api/settings/oauth-providers`      | Administrator view: `providers` and `presets`, without secrets                           |
| PUT    | `/api/settings/oauth-providers`      | Administrator JSON `{providers:[...]}`; replaces the provider list; body limit 128 KiB   |

Profile operations require the current browser session. A provider callback returns a stable result marker in `oauth`
and its ID in `provider`; the SPA translates and removes the markers. Failures never display raw provider responses.
Sessions identify the method as `oauth:<provider-id>`, optionally followed by `+totp` or `+passkey`.

## GitLab Maven authorization

A GitLab.com authorization from the last hour can automatically verify a newly created `io.gitlab.<namespace>` domain.
RenoP accepts the account's own namespace and top-level groups listed in GitLab's group-owner claim. Public group
membership, ownership of a subgroup, and self-hosted GitLab identities do not authorize a parent or GitLab.com
namespace. Refresh the connection to update the ownership proof. Each identity retains at most 1,001 namespaces.
Existing public bio/description verification remains available.

## Security and operation

OAuth callbacks use a ten-minute HttpOnly cookie, single-use server state, and PKCE. At most 2,048 external
authorization states are retained. Configuration changes invalidate pending callbacks and registrations. OIDC verifies
the signature, issuer, audience, subject, nonce, time bounds, and token hash where provided; supported signing
algorithms are RS256 and ES256. Provider responses are limited to 1 MiB and requests have bounded timeouts. The
configured outbound proxy applies.

RenoP stores stable subjects under an authority derived from the provider type, client, endpoints, and verified issuer.
Changing that authority does not transfer existing bindings to a different identity service. Access tokens are not
retained for login. A protected avatar may keep an encrypted token only inside the pending registration until
confirmation or expiry; the private `mfa_encryption_key` protects this temporary value. It is never returned to the
browser.

Account retirement releases every third-party binding atomically. Another live account may bind the released identity,
while the retired username remains reserved permanently, the email remains held for 14 days, and activity retention
remains 30 days. Retired accounts cannot recreate their bindings through a delayed callback or refresh.

Provider identities and every captured contact email must be available together before a new binding can commit. A
released identity cannot bypass another account's retained email ownership.
See [login email aliases](./email-verification.md) for verification, removal, and retention rules.
