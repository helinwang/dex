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

func recoverNtSig(shares []*NtShare) (bls.Sign, error) {
	idVec := make([]bls.ID, len(shares))
	signs := make([]bls.Sign, len(shares))
	for i := range shares {
		var sign bls.Sign
		err := sign.Deserialize(shares[i].SigShare)
		if err != nil {
			return bls.Sign{}, err
		}

		signs[i] = sign
		idVec[i] = shares[i].Owner.ID()
	}

	var sign bls.Sign
	err := sign.Recover(signs, idVec)
	if err != nil {
		return bls.Sign{}, err
	}

	return sign, nil
}

func recoverRandBeaconSig(shares map[Hash]*RandBeaconSigShare) (bls.Sign, error) {
	signs := make([]bls.Sign, len(shares))
	idVec := make([]bls.ID, len(shares))
	i := 0
	for _, s := range shares {
		var sign bls.Sign
		err := sign.Deserialize(s.Share)
		if err != nil {
			return bls.Sign{}, err
		}

		signs[i] = sign
		idVec[i] = s.Owner.ID()
		i++
	}

	var sign bls.Sign
	err := sign.Recover(signs, idVec)
	if err != nil {
		return bls.Sign{}, err
	}

	return sign, nil
}

func randBeaconSigMsg(round int, lastSigHash Hash) []byte {
	var rbs RandBeaconSig
	rbs.LastSigHash = lastSigHash
	rbs.Round = round
	return rbs.Encode(false)
}

func signRandBeaconShare(sk, keyShare bls.SecretKey, round int, lastSigHash Hash) *RandBeaconSigShare {
	msg := randBeaconSigMsg(round, lastSigHash)
	share := keyShare.Sign(string(msg)).Serialize()
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
