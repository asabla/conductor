#!/bin/sh
# =============================================================================
# MinIO Initialization Script
# =============================================================================
# This script initializes MinIO with the required buckets and policies
# for the Conductor test orchestration platform.
#
# Environment variables:
#   MINIO_ROOT_USER     - MinIO root username
#   MINIO_ROOT_PASSWORD - MinIO root password
#   MINIO_BUCKET        - Bucket name to create (default: conductor-artifacts)
# =============================================================================

set -e

# Configuration
MINIO_HOST="${MINIO_HOST:-minio:9000}"
MINIO_ALIAS="conductor"
BUCKET_NAME="${MINIO_BUCKET:-conductor-artifacts}"

echo "=============================================="
echo "MinIO Initialization Script"
echo "=============================================="
echo "Host: ${MINIO_HOST}"
echo "Bucket: ${BUCKET_NAME}"
echo "=============================================="

# Wait for MinIO to be ready
echo "Waiting for MinIO to be ready..."
until mc alias set "${MINIO_ALIAS}" "http://${MINIO_HOST}" "${MINIO_ROOT_USER}" "${MINIO_ROOT_PASSWORD}" > /dev/null 2>&1; do
    echo "MinIO not ready, retrying in 2 seconds..."
    sleep 2
done
echo "MinIO is ready!"

# Create the artifacts bucket if it doesn't exist
echo "Checking bucket: ${BUCKET_NAME}"
if mc ls "${MINIO_ALIAS}/${BUCKET_NAME}" > /dev/null 2>&1; then
    echo "Bucket '${BUCKET_NAME}' already exists"
else
    echo "Creating bucket: ${BUCKET_NAME}"
    mc mb "${MINIO_ALIAS}/${BUCKET_NAME}"
    echo "Bucket '${BUCKET_NAME}' created successfully"
fi

# Set bucket policy for authenticated read access
# This allows agents and control plane to read/write artifacts
echo "Setting bucket policy..."
cat > /tmp/bucket-policy.json << 'EOF'
{
    "Version": "2012-10-17",
    "Statement": [
        {
            "Effect": "Allow",
            "Principal": {"AWS": ["*"]},
            "Action": [
                "s3:GetBucketLocation",
                "s3:ListBucket",
                "s3:ListBucketMultipartUploads"
            ],
            "Resource": ["arn:aws:s3:::BUCKET_NAME"]
        },
        {
            "Effect": "Allow",
            "Principal": {"AWS": ["*"]},
            "Action": [
                "s3:GetObject",
                "s3:PutObject",
                "s3:DeleteObject",
                "s3:ListMultipartUploadParts",
                "s3:AbortMultipartUpload"
            ],
            "Resource": ["arn:aws:s3:::BUCKET_NAME/*"]
        }
    ]
}
EOF

# Replace placeholder with actual bucket name
sed -i "s/BUCKET_NAME/${BUCKET_NAME}/g" /tmp/bucket-policy.json

# Apply the policy
mc anonymous set-json /tmp/bucket-policy.json "${MINIO_ALIAS}/${BUCKET_NAME}"
echo "Bucket policy applied successfully"

# Create directory structure for artifacts
echo "Creating artifact directory structure..."
echo "" | mc pipe "${MINIO_ALIAS}/${BUCKET_NAME}/test-runs/.keep" 2>/dev/null || true
echo "" | mc pipe "${MINIO_ALIAS}/${BUCKET_NAME}/logs/.keep" 2>/dev/null || true
echo "" | mc pipe "${MINIO_ALIAS}/${BUCKET_NAME}/reports/.keep" 2>/dev/null || true

# Set lifecycle policy to auto-delete old artifacts (30 days)
echo "Setting lifecycle policy (30-day retention)..."
cat > /tmp/lifecycle.json << 'EOF'
{
    "Rules": [
        {
            "ID": "expire-old-artifacts",
            "Status": "Enabled",
            "Filter": {
                "Prefix": ""
            },
            "Expiration": {
                "Days": 30
            }
        }
    ]
}
EOF

mc ilm import "${MINIO_ALIAS}/${BUCKET_NAME}" < /tmp/lifecycle.json || echo "Warning: Could not set lifecycle policy (non-fatal)"

# Verify setup
echo ""
echo "=============================================="
echo "MinIO Setup Complete!"
echo "=============================================="
echo "Bucket: ${BUCKET_NAME}"
mc ls "${MINIO_ALIAS}/${BUCKET_NAME}"
echo ""
echo "MinIO Console: http://localhost:9001"
echo "Access Key: ${MINIO_ROOT_USER}"
echo "=============================================="

exit 0
