#!/bin/sh
# Copyright 2020 The Kubernetes Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

set -e

# This has to be set otherwise the default driver will not work
docker buildx create --use --name build --node build --driver-opt network=host

docker buildx build --push --platform linux/${ARCH} \
    -t metalstack/cluster-api-provider-metal:latest-${ARCH} \
    -t metalstack/cluster-api-provider-metal:${TAG}-${ARCH} \
    -f ../../Dockerfile.goreleaser .

# Update the manifest for the new release
docker buildx imagetools create \
  -t metalstack/cluster-api-provider-metal:${TAG} \
  metalstack/cluster-api-provider-metal:${TAG}-${ARCH} \
  metalstack/cluster-api-provider-metal:${TAG}-${ARCH}

docker buildx imagetools create \
  -t metalstack/cluster-api-provider-metal:latest \
  metalstack/cluster-api-provider-metal:latest-${ARCH} \
  metalstack/cluster-api-provider-metal:latest-${ARCH}
