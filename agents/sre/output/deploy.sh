#!/usr/bin/env sh
set -eu

REGISTRY="${REGISTRY:-registry.example.com/ai-interviewer}"
TAG="${TAG:-$(date +%Y%m%d%H%M%S)}"
ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)"

docker build -t "$REGISTRY/model-service:$TAG" "$ROOT/mlengineer/output/model-service"
docker build -t "$REGISTRY/backend-relay:$TAG" "$ROOT/mlengineer/output/backend-relay"

docker push "$REGISTRY/model-service:$TAG"
docker push "$REGISTRY/backend-relay:$TAG"

printf '%s\n' "Pushed:"
printf '%s\n' "$REGISTRY/model-service:$TAG"
printf '%s\n' "$REGISTRY/backend-relay:$TAG"
