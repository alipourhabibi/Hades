#!/bin/bash

set -e

DOMAIN="example.com"
PORT=443
GRPC_SERVER="$DOMAIN:$PORT"
NETRC_FILE="$HOME/.netrc"
AUTH_SERVICE="hades.api.authentication.v1.AuthenticationService"
MODULE_SERVICE="hades.api.registry.v1.ModuleService"

login_request() {
    cat <<EOF
{
  "username": "googleapis",
  "password": "googleapis"
}
EOF
}

signin_request() {
    cat <<EOF
{
  "username": "googleapis",
  "password": "googleapis",
  "email": "googleapis@example.com",
  "description": "googleapis"
}
EOF
}

SIGNIN_RESPONSE=$(grpcurl -d "$(signin_request)" "$GRPC_SERVER" "$AUTH_SERVICE.Signin") || true

TOKEN=$(grpcurl -d "$(login_request)" "$GRPC_SERVER" "$AUTH_SERVICE.Login" | jq -r .token)
if [[ -z "$TOKEN" ]]; then
    echo "Error: Unable to retrieve token."
    exit 1
fi
if grep -q "machine $DOMAIN" $NETRC_FILE; then
    sed -i "/machine $DOMAIN/{N;N;s/password .*/password $TOKEN/;}" $NETRC_FILE
    echo "Token updated successfully."
else
    cat >> $NETRC_FILE <<EOF
machine $DOMAIN
  login googleapis
  password $TOKEN
EOF
    echo "Token added for machine $DOMAIN."
fi

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

grpcurl -H "Authorization: Bearer $TOKEN" -d "$(create_module_request)" "$GRPC_SERVER" "$MODULE_SERVICE.CreateModuleByName"

cd protos/googleapis && buf push && cd -
cd protos/simpleproject && buf dep update && cd -
