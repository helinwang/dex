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

func signRandBeaconShare(sk, keyShare bls.SecretKey, round int, lastSigHash Hash) *RandBeaconSigShare {
	share := keyShare.Sign(string(lastSigHash[:])).Serialize()
	s := &RandBeaconSigShare{
		Owner:       hash(sk.GetPublicKey().Serialize()).Addr(),
		Round:       round,
		LastSigHash: lastSigHash,
		Share:       share,
	}

	sig := sk.Sign(string(s.Encode(false))).Serialize()
	s.OwnerSig = sig
	return s
}
