#!/bin/bash

set -e

DOMAIN="example.com"
PORT=443
GRPC_SERVER="$DOMAIN:$PORT"
NETRC_FILE="$HOME/.netrc"
AUTH_SERVICE="hades.api.authentication.v1.AuthenticationService"
MODULE_SERVICE="hades.api.registry.v1.ModuleService"

USERNAME="googleapis"
PASSWORD="googleapis!@#123"
EMAIL="googleapis@example.com"

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

signin_request() {
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

auth_user() {
    local username="$1"
    local password="$2"
    local email="$3"

    # signup (ignore if exists)
    grpcurl -d "$(signin_request "$username" "$password" "$email")" \
        "$GRPC_SERVER" "$AUTH_SERVICE.Signin" || true

    # login
    grpcurl -d "$(login_request "$username" "$password")" \
        "$GRPC_SERVER" "$AUTH_SERVICE.Login" | jq -r .token
}

update_netrc() {
    local domain="$1"
    local login="$2"
    local token="$3"
    local file="${4:-$HOME/.netrc}"

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

GOOGLE_TOKEN=$(auth_user "googleapis" "googleapis!@#123" "googleapis@example.com")
USER_TOKEN=$(auth_user "someuser" "somepass!@#123" "someuser@example.com")

if [[ -z "$GOOGLE_TOKEN" || -z "$USER_TOKEN" ]]; then
    echo "Error: failed to authenticate users"
    exit 1
fi

update_netrc $DOMAIN "googleapis" "$GOOGLE_TOKEN"

grpcurl -H "Authorization: Bearer $GOOGLE_TOKEN" -d "$(create_module_request)" "$GRPC_SERVER" "$MODULE_SERVICE.CreateModuleByName"
cd protos/googleapis && buf push; cd -

update_netrc $DOMAIN "someuser" "$USER_TOKEN"

cd protos/simpleproject && buf dep update && cd -
