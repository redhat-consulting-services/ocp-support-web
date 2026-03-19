#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
NAMESPACE="ocp-support-web"

# Image defaults — override these for disconnected / mirror registries
BUILDER_IMAGE="${BUILDER_IMAGE:-registry.redhat.io/ubi9/go-toolset:latest}"
RUNTIME_IMAGE="${RUNTIME_IMAGE:-registry.redhat.io/openshift4/ose-cli-rhel9:latest}"
OAUTH_PROXY_IMAGE="${OAUTH_PROXY_IMAGE:-registry.redhat.io/openshift4/ose-oauth-proxy-rhel9:latest}"
DEFAULT_MUST_GATHER_IMAGE="${MUST_GATHER_IMAGE_DEFAULT:-}"
CNV_MUST_GATHER_IMAGE="${MUST_GATHER_IMAGE_CNV:-registry.redhat.io/container-native-virtualization/cnv-must-gather-rhel9:v4.17.0}"
ODF_MUST_GATHER_IMAGE="${MUST_GATHER_IMAGE_ODF:-registry.redhat.io/odf4/ocs-must-gather-rhel9:latest}"

echo "=== OCP Support Web Deployment ==="
echo ""

# Check oc is available and logged in
if ! oc whoami &>/dev/null; then
    echo "ERROR: Not logged in to OpenShift. Run 'oc login' first."
    exit 1
fi

echo "Logged in as: $(oc whoami)"
echo "Cluster: $(oc whoami --show-server)"
echo ""

# Detect cluster domain
CLUSTER_DOMAIN=$(oc get ingresses.config/cluster -o jsonpath='{.spec.domain}' 2>/dev/null || true)
if [[ -z "$CLUSTER_DOMAIN" ]]; then
    echo "ERROR: Could not detect cluster domain. Ensure you have cluster-admin access."
    exit 1
fi
echo "Cluster domain: $CLUSTER_DOMAIN"
echo ""
echo "Images:"
echo "  Builder:          $BUILDER_IMAGE"
echo "  Runtime:          $RUNTIME_IMAGE"
echo "  OAuth Proxy:      $OAUTH_PROXY_IMAGE"
echo "  CNV Must-Gather:  $CNV_MUST_GATHER_IMAGE"
echo "  ODF Must-Gather:  $ODF_MUST_GATHER_IMAGE"
echo ""

# Generate cookie secret for oauth-proxy
COOKIE_SECRET=$(head -c 32 /dev/urandom | base64 | tr -d '\n' | head -c 32)

# Step 1: Create namespace
echo "--- Creating namespace..."
oc apply -f "$SCRIPT_DIR/deploy/namespace.yaml"

# Step 2: Create service account
echo "--- Creating service account..."
oc apply -f "$SCRIPT_DIR/deploy/serviceaccount.yaml"

# Step 3: Bind cluster-admin
echo "--- Binding cluster-admin to service account..."
oc apply -f "$SCRIPT_DIR/deploy/clusterrolebinding.yaml"

# Step 4: Create image stream and build config
echo "--- Creating image stream and build config..."
oc apply -f "$SCRIPT_DIR/deploy/imagestream.yaml"
cat "$SCRIPT_DIR/deploy/buildconfig.yaml" \
    | sed "s|BUILDER_IMAGE_PLACEHOLDER|${BUILDER_IMAGE}|g" \
    | sed "s|RUNTIME_IMAGE_PLACEHOLDER|${RUNTIME_IMAGE}|g" \
    | oc apply -f -

# Step 5: Build the image
echo "--- Starting build (uploading source)..."
oc start-build ocp-support-web \
    --from-dir="$SCRIPT_DIR" \
    -n "$NAMESPACE" \
    --follow

# Step 6: Get the built image reference
IMAGE_REF="image-registry.openshift-image-registry.svc:5000/${NAMESPACE}/ocp-support-web:latest"
echo "Built image: $IMAGE_REF"

# Step 7: Create service
echo "--- Creating service..."
oc apply -f "$SCRIPT_DIR/deploy/service.yaml"

# Step 8: Deploy with actual image, cluster domain, and cookie secret
echo "--- Creating deployment..."
cat "$SCRIPT_DIR/deploy/deployment.yaml" \
    | sed "s|IMAGE_PLACEHOLDER|${IMAGE_REF}|g" \
    | sed "s|OAUTH_PROXY_IMAGE_PLACEHOLDER|${OAUTH_PROXY_IMAGE}|g" \
    | sed "s|CLUSTER_DOMAIN_PLACEHOLDER|${CLUSTER_DOMAIN}|g" \
    | sed "s|COOKIE_SECRET_PLACEHOLDER|${COOKIE_SECRET}|g" \
    | sed "s|DEFAULT_MG_IMAGE_PLACEHOLDER|${DEFAULT_MUST_GATHER_IMAGE}|g" \
    | sed "s|CNV_IMAGE_PLACEHOLDER|${CNV_MUST_GATHER_IMAGE}|g" \
    | sed "s|ODF_IMAGE_PLACEHOLDER|${ODF_MUST_GATHER_IMAGE}|g" \
    | oc apply -f -

# Step 9: Create route
echo "--- Creating route..."
oc apply -f "$SCRIPT_DIR/deploy/route.yaml"

# Wait for rollout
echo "--- Waiting for deployment to be ready..."
oc rollout status deployment/ocp-support-web -n "$NAMESPACE" --timeout=120s || true

# Get route URL
ROUTE_URL=$(oc get route ocp-support-web -n "$NAMESPACE" -o jsonpath='{.spec.host}' 2>/dev/null || true)

echo ""
echo "=== Deployment complete ==="
echo ""
echo "  Namespace:       $NAMESPACE"
echo "  Service Account: ocp-support-web (cluster-admin)"
echo "  Auth:            OpenShift OAuth (cluster-admin users only)"
echo "  Route:           https://$ROUTE_URL"
echo ""
echo "Only users with cluster-admin privileges can access this application."
echo ""
echo "For disconnected environments, set these env vars before running this script:"
echo "  BUILDER_IMAGE          - Go build image (default: registry.redhat.io/ubi9/go-toolset:latest)"
echo "  RUNTIME_IMAGE          - Runtime image with oc CLI (default: registry.redhat.io/openshift4/ose-cli-rhel9:latest)"
echo "  OAUTH_PROXY_IMAGE      - OAuth proxy sidecar (default: registry.redhat.io/openshift4/ose-oauth-proxy-rhel9:latest)"
echo "  MUST_GATHER_IMAGE_CNV  - CNV must-gather image (default: registry.redhat.io/container-native-virtualization/cnv-must-gather-rhel9:v4.17.0)"
echo "  MUST_GATHER_IMAGE_ODF  - ODF must-gather image (default: registry.redhat.io/odf4/ocs-must-gather-rhel9:latest)"
