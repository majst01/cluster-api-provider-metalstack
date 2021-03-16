# Set env variables
DEFAULT_KUBERNETES_VERSION=1.20.3
DEFAULT_POD_CIDR="172.25.0.0/16"
DEFAULT_SERVICE_CIDR="172.26.0.0/16"
DEFAULT_MASTER_NODE_TYPE="v1-small-x86"
DEFAULT_WORKER_NODE_TYPE="v1-small-x86"
DEFAULT_NODE_IMAGE="ubuntu-cloud-init-20.04"
DEFAULT_CONTROL_PLANE_IP="100.255.254.3"

CLUSTER_NAME=${CLUSTER_NAME:-${DEFAULT_CLUSTER_NAME}}
POD_CIDR=${POD_CIDR:-${DEFAULT_POD_CIDR}}
SERVICE_CIDR=${SERVICE_CIDR:-${DEFAULT_SERVICE_CIDR}}
WORKER_NODE_TYPE=${WORKER_NODE_TYPE:-${DEFAULT_WORKER_NODE_TYPE}}
MASTER_NODE_TYPE=${MASTER_NODE_TYPE:-${DEFAULT_MASTER_NODE_TYPE}}
NODE_IMAGE=${DEFAULT_NODE_IMAGE:-${DEFAULT_NODE_IMAGE}}
CONTROL_PLANE_IP=${DEFAULT_CONTROL_PLANE_IP:-${DEFAULT_CONTROL_PLANE_IP}}
KUBERNETES_VERSION=${KUBERNETES_VERSION:-${DEFAULT_KUBERNETES_VERSION}}
SSH_KEY=${SSH_KEY:-""}
USE_EXISTING_CLUSTER=true

NETWORK_ID=${METAL_NETWORK_ID:-"00000000-0000-0000-0000-000000000000"}
PROJECT_ID=${METAL_PROJECT_ID:-"00000000-0000-0000-0000-000000000000"}
PARTITION=${METAL_PARTITION:-"vagrant"}

NODE_OS=NODE_IMAGE
FACILITY=PARTITION

# Connect docker registry to kind cluster
docker network connect "kind" "kind-registry" || true
cat <<EOF | kubectl apply -f -
apiVersion: v1
kind: ConfigMap
metadata:
  name: local-registry-hosting
  namespace: kube-public
data:
  localRegistryHosting.v1: |
    host: "localhost:5000"
    help: "https://kind.sigs.k8s.io/docs/user/local-registry/"
EOF

# Tag and push provider image to registry
docker tag metalstack/cluster-api-provider-metalstack:latest localhost:5000/cluster-api-provider-metalstack:latest
docker push localhost:5000/cluster-api-provider-metalstack:latest

export PROJECT_ID PARTITION NETWORK_ID NODE_IMAGE CONTROL_PLANE_IP WORKER_NODE_TYPE MASTER_NODE_TYPE POD_CIDR SERVICE_CIDR SSH_KEY USE_EXISTING_CLUSTER KUBERNETES_VERSION NODE_OS FACILITY
make e2e-test