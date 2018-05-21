package consensus

import (
	"log"

	"github.com/dfinity/go-dfinity-crypto/bls"
)

func verifySig(pk bls.PublicKey, sig []byte, msg []byte) bool {
	if len(sig) == 0 {
		return false
	}

	var sign bls.Sign
	err := sign.Deserialize(sig)
	if err != nil {
		log.Printf("verify sig error: %v\n", err)
		return false
	}

	return sign.Verify(&pk, string(msg))
}

func recoverNtSig(shares []*NtShare) bls.Sign {
	// TODO
	return bls.Sign{}
}

func recoverRandBeaconSig(shares []*RandBeaconSigShare) bls.Sign {
	// TODO
	return bls.Sign{}
}
