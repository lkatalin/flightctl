#!/usr/bin/env python3
"""
Mock Keylime Verifier for FlightCTL attestation testing.

This is a simple one-shot verifier that:
1. Accepts POST requests to /v2.1/agents/{agent_id}
2. Validates that required fields are present (aik_tpm)
3. Returns a success or failure response
4. Does not maintain any state

This is intended for development and testing only.
"""

from flask import Flask, request, jsonify
import logging
import sys

app = Flask(__name__)

# Configure logging
logging.basicConfig(
    level=logging.INFO,
    format='%(asctime)s - %(levelname)s - %(message)s',
    stream=sys.stdout
)
logger = logging.getLogger(__name__)


@app.route('/v2.1/agents/<agent_id>', methods=['POST'])
def verify_attestation(agent_id):
    """
    Verify TPM attestation data (one-shot, no state maintained).

    Expected request body:
    {
        "aik_tpm": "base64-encoded AIK public key",
        "ek_tpm": "base64-encoded EK public key (optional)",
        "mb_policy": "JSON measured boot policy (optional)",
        "runtime_policy": "JSON IMA runtime policy (optional)",
        "tpm_policy": "JSON TPM PCR policy (optional)"
    }

    Returns:
    {
        "results": "Success" or error message
    }
    """
    logger.info(f"Received attestation verification request for agent: {agent_id}")

    # Get request JSON
    data = request.get_json()
    if not data:
        logger.error("No JSON body in request")
        return jsonify({
            "code": 400,
            "status": "Bad Request",
            "results": {"error": "No JSON body provided"}
        }), 400

    # Check for required field
    if 'aik_tpm' not in data:
        logger.error("Missing required field: aik_tpm")
        return jsonify({
            "code": 400,
            "status": "Bad Request",
            "results": {"error": "Missing required field: aik_tpm"}
        }), 400

    # Log received data (truncated for readability)
    aik_len = len(data.get('aik_tpm', ''))
    ek_len = len(data.get('ek_tpm', '')) if data.get('ek_tpm') else 0
    has_mb_policy = 'mb_policy' in data and data['mb_policy']
    has_runtime_policy = 'runtime_policy' in data and data['runtime_policy']
    has_tpm_policy = 'tpm_policy' in data and data['tpm_policy']

    logger.info(f"  Agent ID: {agent_id}")
    logger.info(f"  AIK length: {aik_len} bytes")
    logger.info(f"  EK length: {ek_len} bytes")
    logger.info(f"  Has MB policy: {has_mb_policy}")
    logger.info(f"  Has runtime policy: {has_runtime_policy}")
    logger.info(f"  Has TPM policy: {has_tpm_policy}")

    # Simulate verification logic
    # In a real verifier, this would:
    # 1. Validate the AIK and EK certificates
    # 2. Verify the TPM quote against expected PCR values
    # 3. Check measured boot log against policy
    # 4. Verify IMA runtime measurements against policy

    # For this mock, we always return success
    # You could add logic here to fail certain agents for testing
    logger.info(f"Attestation verification PASSED for agent: {agent_id}")

    return jsonify({
        "results": "Success"
    }), 200


@app.route('/health', methods=['GET'])
def health():
    """Health check endpoint."""
    return jsonify({"status": "healthy"}), 200


if __name__ == '__main__':
    logger.info("Starting Keylime Verifier Mock")
    logger.info("This is a test/development verifier - NOT for production use")
    logger.info("Listening on 0.0.0.0:8881")

    # Run the Flask app
    app.run(host='0.0.0.0', port=8881, debug=False)
