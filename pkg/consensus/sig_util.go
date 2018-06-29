package consensus

import (
	"github.com/dfinity/go-dfinity-crypto/bls"
)

func recoverNtSig(shares []*NtShare) (Sig, error) {
	idVec := make([]bls.ID, len(shares))
	signs := make([]bls.Sign, len(shares))
	for i := range shares {
		var sign bls.Sign
		err := sign.Deserialize(shares[i].SigShare)
		if err != nil {
			return nil, err
		}

		signs[i] = sign
		idVec[i] = shares[i].Owner.ID()
	}

	var sign bls.Sign
	err := sign.Recover(signs, idVec)
	if err != nil {
		return nil, err
	}

	return Sig(sign.Serialize()), nil
}

func recoverRandBeaconSig(shares []*RandBeaconSigShare) (Sig, error) {
	signs := make([]bls.Sign, len(shares))
	idVec := make([]bls.ID, len(shares))
	i := 0
	for _, s := range shares {
		var sign bls.Sign
		err := sign.Deserialize(s.Share)
		if err != nil {
			return nil, err
		}

		signs[i] = sign
		idVec[i] = s.Owner.ID()
		i++
	}

	var sign bls.Sign
	err := sign.Recover(signs, idVec)
	if err != nil {
		return nil, err
	}

	return Sig(sign.Serialize()), nil
}

func randBeaconSigMsg(round uint64, lastSigHash Hash) []byte {
	var rbs RandBeaconSig
	rbs.LastSigHash = lastSigHash
	rbs.Round = round
	return rbs.Encode(false)
}

func signRandBeaconSigShare(sk, keyShare SK, round uint64, lastSigHash Hash) *RandBeaconSigShare {
	msg := randBeaconSigMsg(round, lastSigHash)
	share := keyShare.Sign(msg)
	s := &RandBeaconSigShare{
		Owner:       sk.MustPK().Addr(),
		Round:       round,
		LastSigHash: lastSigHash,
		Share:       share,
	}

	s.OwnerSig = sk.Sign(s.Encode(false))
	return s
}
