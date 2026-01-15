#!/bin/bash
set -e

# Create a test swtpm CA for TPM testing
# WARNING: This is for testing only! DO NOT use in production.

CA_DIR="${1:-bin/swtpm-ca}"

echo "Creating test swtpm CA in $CA_DIR..."
mkdir -p "$CA_DIR"

# Check if CA already exists
if [ -f "$CA_DIR/signkey.pem" ] && [ -f "$CA_DIR/issuercert.pem" ] && [ -f "$CA_DIR/swtpm-localca-rootca-cert.pem" ]; then
    echo "Test swtpm CA already exists, skipping creation"
    exit 0
fi

# Generate root CA private key
openssl genrsa -out "$CA_DIR/swtpm-localca-rootca-privkey.pem" 2048 2>/dev/null

# Generate root CA certificate (self-signed, valid for 10 years)
openssl req -new -x509 \
    -key "$CA_DIR/swtpm-localca-rootca-privkey.pem" \
    -out "$CA_DIR/swtpm-localca-rootca-cert.pem" \
    -days 3650 \
    -subj "/CN=swtpm-localca-rootca" \
    -extensions v3_ca \
    -config <(cat <<EOF
[ req ]
distinguished_name = req_distinguished_name
x509_extensions = v3_ca

[ req_distinguished_name ]

[ v3_ca ]
basicConstraints = critical,CA:TRUE
keyUsage = critical,keyCertSign,cRLSign
subjectKeyIdentifier = hash
EOF
) 2>/dev/null

# Generate intermediate CA (issuer) private key
openssl genrsa -out "$CA_DIR/signkey.pem" 2048 2>/dev/null

# Create CSR for intermediate CA
openssl req -new \
    -key "$CA_DIR/signkey.pem" \
    -out "$CA_DIR/issuercert.csr" \
    -subj "/CN=swtpm-localca" 2>/dev/null

# Sign intermediate CA with root CA
openssl x509 -req \
    -in "$CA_DIR/issuercert.csr" \
    -CA "$CA_DIR/swtpm-localca-rootca-cert.pem" \
    -CAkey "$CA_DIR/swtpm-localca-rootca-privkey.pem" \
    -CAcreateserial \
    -out "$CA_DIR/issuercert.pem" \
    -days 3650 \
    -extensions v3_intermediate \
    -extfile <(cat <<EOF
[ v3_intermediate ]
basicConstraints = critical,CA:TRUE,pathlen:0
keyUsage = critical,keyCertSign,cRLSign
subjectKeyIdentifier = hash
authorityKeyIdentifier = keyid:always
EOF
) 2>/dev/null

# Initialize certificate serial number file
echo "01" > "$CA_DIR/certserial"

# Clean up temporary files
rm -f "$CA_DIR/issuercert.csr" "$CA_DIR/swtpm-localca-rootca-cert.srl"

echo "✓ Created test swtpm CA:"
echo "  Root CA:        $CA_DIR/swtpm-localca-rootca-cert.pem"
echo "  Intermediate:   $CA_DIR/issuercert.pem"
echo "  Signing key:    $CA_DIR/signkey.pem"
echo ""
echo "⚠️  WARNING: This is a TEST CA only! Never use in production."
