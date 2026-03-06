# Dockerfile to build Keylime verifier from latest upstream/master
# This ensures we have the most recent changes including v2.4 API updates

FROM registry.fedoraproject.org/fedora:latest

# Install dependencies
RUN dnf install -y \
    python3-pip \
    python3-devel \
    python3-gpg \
    git \
    gcc \
    make \
    openssl \
    openssl-devel \
    procps-ng \
    tpm2-tools \
    && dnf clean all

# Clone latest Keylime from upstream master
RUN git clone https://github.com/keylime/keylime.git /keylime

WORKDIR /keylime

# Install Keylime's Python dependencies from requirements.txt
RUN pip3 install --no-cache-dir -r requirements.txt

# Install Keylime itself in editable mode
RUN pip3 install --no-cache-dir -e .

# Create directory for TLS certificates
RUN mkdir -p /var/lib/keylime/cv_ca

# Expose verifier port
EXPOSE 8881

# Run Keylime verifier
CMD ["keylime_verifier"]
