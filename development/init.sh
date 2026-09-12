#!/bin/bash

# Bootstrap a development registry: two users, one module, one push.
#
# Two things it has to get right that it used to get wrong:
#
#   1. Login refuses an account whose email is unverified, and with
#      email.stub: true nothing is ever delivered, so there is no link for
#      anyone to click. config/dev.yaml sets auth.emailVerification.autoVerify,
#      which marks a new address verified at registration. That setting is for
#      development only; the check below fails loudly rather than silently if it
#      is off, because the alternative is an opaque "failed to authenticate".
#
#   2. `buf push` refuses an interactive session token ("this operation
#      requires an API token, not an interactive session"), which is a good
#      decision and predates this script. So the netrc gets an API token minted
#      through APITokenService.CreateAPIToken, not the login token.

set -e

DOMAIN="${HADES_DOMAIN:-localhost}"
PORT="${HADES_PORT:-50051}"
PLAINTEXT="${HADES_PLAINTEXT:-true}"
GRPC_SERVER="$DOMAIN:$PORT"
NETRC_FILE="$HOME/.netrc"
AUTH_SERVICE="hades.api.auth.v1.AuthenticationService"
TOKEN_SERVICE="hades.api.auth.v1.APITokenService"
MODULE_SERVICE="hades.api.registry.v1.ModuleService"

GRPCURL_FLAGS=()
if [[ "$PLAINTEXT" == "true" ]]; then
    GRPCURL_FLAGS+=("-plaintext")
fi

login_request() {
    local username="$1"
    local password="$2"

    cat <<EOF
{
  "username": "$username",
  "password": "$password"
}
EOF
}

register_request() {
    local username="$1"
    local password="$2"
    local email="$3"

    cat <<EOF
{
  "username": "$username",
  "password": "$password",
  "email": "$email",
  "description": "$username"
}
EOF
}

# session_token registers the user if needed and returns a session token.
session_token() {
    local username="$1"
    local password="$2"
    local email="$3"

    # Registration is best effort: an existing account is the normal case on a
    # re-run and reports AlreadyExists.
    grpcurl "${GRPCURL_FLAGS[@]}" -d "$(register_request "$username" "$password" "$email")" \
        "$GRPC_SERVER" "$AUTH_SERVICE.Register" >/dev/null 2>&1 || true

    grpcurl "${GRPCURL_FLAGS[@]}" -d "$(login_request "$username" "$password")" \
        "$GRPC_SERVER" "$AUTH_SERVICE.Login" | jq -r .token
}

# api_token mints a personal API token, which is the credential `buf push`
# accepts. It takes a session token, because minting one is a session-only
# operation: a leaked API token must not be able to mint another.
api_token() {
    local session="$1"
    local name="$2"

    grpcurl "${GRPCURL_FLAGS[@]}" -H "Authorization: Bearer $session" \
        -d "{\"name\": \"$name\"}" \
        "$GRPC_SERVER" "$TOKEN_SERVICE.CreateAPIToken" | jq -r .token
}

update_netrc() {
    local domain="$1"
    local login="$2"
    local token="$3"
    local file="${4:-$HOME/.netrc}"

    touch "$file"
    if grep -q "^machine $domain" "$file"; then
        awk -v domain="$domain" -v login="$login" -v token="$token" '
        BEGIN { updated=0 }
        $1 == "machine" && $2 == domain {
            print "machine " domain
            print "  login " login
            print "  password " token
            updated=1
            skip=1
            next
        }
        skip && ($1 == "machine") { skip=0 }
        !skip
        END {
            if (!updated) exit 1
        }' "$file" > "$file.tmp" && mv "$file.tmp" "$file"
    else
        cat >> "$file" <<EOF
machine $domain
  login $login
  password $token
EOF
    fi
}

create_module_request() {
    cat <<EOF
{
    "default_branch": "googleapis",
    "description": "googleapis module",
    "name": "googleapis",
    "visibility": 1
}
EOF
}

GOOGLE_SESSION=$(session_token "googleapis" "googleapis!@#123" "googleapis@example.com")
USER_SESSION=$(session_token "someuser" "somepass!@#123" "someuser@example.com")

if [[ -z "$GOOGLE_SESSION" || "$GOOGLE_SESSION" == "null" || -z "$USER_SESSION" || "$USER_SESSION" == "null" ]]; then
    echo "Error: failed to authenticate users." >&2
    echo "The usual cause is that the accounts exist but their addresses are not verified." >&2
    echo "Set auth.emailVerification.autoVerify: true in your config (development only)," >&2
    echo "or verify the addresses through AuthenticationService.VerifyEmail." >&2
    exit 1
fi

GOOGLE_TOKEN=$(api_token "$GOOGLE_SESSION" "init.sh")
USER_TOKEN=$(api_token "$USER_SESSION" "init.sh")

if [[ -z "$GOOGLE_TOKEN" || "$GOOGLE_TOKEN" == "null" || -z "$USER_TOKEN" || "$USER_TOKEN" == "null" ]]; then
    echo "Error: failed to mint API tokens. buf push will not accept a session token." >&2
    exit 1
fi

grpcurl "${GRPCURL_FLAGS[@]}" -H "Authorization: Bearer $GOOGLE_SESSION" -d "$(create_module_request)" "$GRPC_SERVER" "$MODULE_SERVICE.CreateModuleByName"

update_netrc "$DOMAIN" "googleapis" "$GOOGLE_TOKEN"
cd protos/googleapis && buf push; cd -

update_netrc "$DOMAIN" "someuser" "$USER_TOKEN"
cd protos/simpleproject && buf dep update && cd -
