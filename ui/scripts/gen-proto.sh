#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
UI_DIR="$(dirname "$SCRIPT_DIR")"
PROTO_ROOT="$UI_DIR/../proto"
OUT_DIR="$UI_DIR/src/gen"
PLUGIN="$UI_DIR/node_modules/.bin/protoc-gen-ts_proto"

if [ ! -f "$PLUGIN" ]; then
  echo "ts-proto plugin not found — run npm install first" >&2
  exit 1
fi

mkdir -p "$OUT_DIR"

TS_PROTO_OPTS="env=browser,outputClientImpl=grpc-web,esModuleInterop=true,stringEnums=true,useOptionals=messages"

# registry pulls in secwager/common enums via import, so compile it alongside
protoc \
  --plugin="$PLUGIN" \
  --ts_proto_out="$OUT_DIR" \
  --ts_proto_opt="$TS_PROTO_OPTS" \
  -I "$PROTO_ROOT" \
  registry/registry.proto

protoc \
  --plugin="$PLUGIN" \
  --ts_proto_out="$OUT_DIR" \
  --ts_proto_opt="$TS_PROTO_OPTS" \
  -I "$PROTO_ROOT" \
  cashier/cashier.proto

protoc \
  --plugin="$PLUGIN" \
  --ts_proto_out="$OUT_DIR" \
  --ts_proto_opt="$TS_PROTO_OPTS" \
  -I "$PROTO_ROOT" \
  userregistration/userregistration.proto

echo "proto codegen complete → $OUT_DIR"
