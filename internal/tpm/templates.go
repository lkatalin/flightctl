package tpm

import (
	//"encoding/binary"
	/*
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/google/go-tpm-tools/client"
	pbattest "github.com/google/go-tpm-tools/proto/attest"
	pbtpm "github.com/google/go-tpm-tools/proto/tpm"*/
	//legacy "github.com/google/go-tpm/legacy/tpm2"
	//"github.com/google/go-tpm-tools/client"
	"github.com/google/go-tpm/tpm2"
	//"github.com/google/go-tpm/tpmutil"
)

// type EK template - already taken care of in go-tpm-tools

//type EndorsementAKTemplate struct {

//}

// we are converting from https://github.com/google/go-tpm/blob/d88acdb2077207b23d3b5dacc2873169283cf11c/legacy/tpm2/structures.go#L936 to
// https://github.com/google/go-tpm/blob/d88acdb2077207b23d3b5dacc2873169283cf11c/tpm2/structures.go#L513
/*func ToTPM2BData(n legacy.Name) *tpm2.TPM2BData {
	if n.Handle != nil {	
		// no idea if this is correct, how long it's supposed to be, or whether it actually is big endian
		byteSlice := make([]byte, 4)
		binary.BigEndian.PutUint32(byteSlice, n.Handle.HandleValue())
		
		// uses https://github.com/google/go-tpm/blob/d88acdb2077207b23d3b5dacc2873169283cf11c/tpm2/structures.go#L124
		return &tpm2.TPM2BData{
			Buffer: byteSlice,
		}
	} else if n.Digest != nil {
		// uses https://github.com/google/go-tpm/blob/d88acdb2077207b23d3b5dacc2873169283cf11c/legacy/tpm2/structures.go#L1025
		bytes, err := n.Digest.Encode()
		if err != nil {
			return nil
		}
		return &tpm2.TPM2BData{
			Buffer: bytes,
		}
	}
	return nil
}*/


// See also - the EK template:
// https://github.com/google/go-tpm/blob/d88acdb2077207b23d3b5dacc2873169283cf11c/tpm2/templates.go#L146
// ... that one also has an AuthPolicy, which this doesn't (maybe not needed?)

// TODO - how do we access the IDevID parent?
func CreateEndorsementLDevIDCreateTemplate(ek tpm2.CreatePrimaryResponse) tpm2.Create {
	//handle := ToTPM2Handle(ek.Handle().HandleValue())
	return tpm2.Create{
		ParentHandle: tpm2.NamedHandle{
				//Handle: tpm2.TPMHandle(ek.Handle().HandleValue()),
				//Name:   tpm2.TPM2BName(*ToTPM2BData(ek.Name())),
				Handle: ek.ObjectHandle,
				Name: ek.Name,
			},
		InPublic: tpm2.New2B(tpm2.TPMTPublic{
            		Type:    tpm2.TPMAlgECC,
            		NameAlg: tpm2.TPMAlgSHA256,
            		ObjectAttributes: tpm2.TPMAObject{
				// see section 3.9 of https://trustedcomputinggroup.org/wp-content/uploads/TCG_IWG_DevID_v1r2_02dec2020.pdf
                		FixedTPM:             true, //must stay in TPM
                		STClear:              true, //cannot be loaded after tpm2_clear - on the ECCEKTemplate this is false?
                		FixedParent:          true, //can't be re-parented
                		SensitiveDataOrigin:  true, //TPM generates all sensitive data during creation
                		UserWithAuth:         true, //true means there are more options for the user to auth - on the ECCEKTemplate this is false
                		AdminWithPolicy:      false, //false means there are more options for admin - on the ECCEKTemplate this is true
                		NoDA:                 true, //true means there are dictionary attack protections - on the ECCEKTemplate this is false
                		EncryptedDuplication: true, //true means there are more robust protections for duplication - on the ECCEKTemplate this is false
                		Restricted:           false, //false means can be used to sign data from outside tpm
                		Decrypt:              false, //can be used to decrypt
                		SignEncrypt:          true, //for asymm, may be used to sign
                		X509Sign:             false, //false means the key can be used to sign if sign is SET (?) - not present in ECCEKTemplate
            		},
            		Parameters: tpm2.NewTPMUPublicParms(
                		tpm2.TPMAlgECC,
                		&tpm2.TPMSECCParms{
					// is this the right way to set this? can't make the TPMTSymDefObject == AlgNull despite
					// https://github.com/google/go-tpm/blob/d88acdb2077207b23d3b5dacc2873169283cf11c/tpm2/structures.go#L2769
                    			Symmetric: tpm2.TPMTSymDefObject{
						Algorithm: tpm2.TPMAlgNull, // because it's not a restricted decryption key
						KeyBits: tpm2.NewTPMUSymKeyBits(
							tpm2.TPMAlgNull,
							tpm2.TPMKeyBits(128),
						),
						Mode: tpm2.NewTPMUSymMode(
							tpm2.TPMAlgNull,
							tpm2.TPMAlgCFB,
						),
					},
                    			Scheme: tpm2.TPMTECCScheme{
                        			Scheme: tpm2.TPMAlgECDSA,
                        			Details: tpm2.NewTPMUAsymScheme(
                                			tpm2.TPMAlgECDSA,
                                			&tpm2.TPMSSigSchemeECDSA{
                                    			HashAlg: tpm2.TPMAlgSHA256,
                                			},
                            			),
                    			},
                    			CurveID: tpm2.TPMECCNistP256,
					// also should be set to NULL according to
					// https://github.com/google/go-tpm/blob/d88acdb2077207b23d3b5dacc2873169283cf11c/tpm2/structures.go#L2779
                    			KDF: tpm2.TPMTKDFScheme{
						Scheme: tpm2.TPMAlgNull,
					},
                		},
            		),
            		Unique: tpm2.NewTPMUPublicID(
                		tpm2.TPMAlgECC,
                		&tpm2.TPMSECCPoint{
                    			X: tpm2.TPM2BECCParameter{Buffer: make([]byte, 32)},
                    			Y: tpm2.TPM2BECCParameter{Buffer: make([]byte, 32)},
                		},
            		),
        	}),
	}
}

/*func ToTPM2Handle(h uint32) tpm2.TPMHandle {
	switch h {
	case 0x40000001:
		return tpm2.TPMRHOwner
	case 0x40000007:
		return tpm2.TPMRHNull
	case 0x40000009:
		return tpm2.TPMRSPW
	case 0x4000000A:
		return tpm2.TPMRHLockout
	case 0x4000000B:
		return tpm2.TPMRHEndorsement
	case 0x4000000C:
		return tpm2.TPMRHPlatform
	case 0x4000000D:
		return tpm2.TPMRHPlatformNV
	/*case 0x40000140:
		return tpm2.TPMRHFWOwner
	case 0x40000141:
		return tpm2.TPMRHFWEndorsement
	case 0x40000142:
		return tpm2.TPMRHFWPlatform
	case 0x40000143:
		return tpm2.TPMRHFWNull
	}
	return nil
}*/
