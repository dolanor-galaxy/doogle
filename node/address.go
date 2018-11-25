package node

import (
	"crypto/rand"
	"crypto/sha1"

	"github.com/pkg/errors"
)

const (
	addressLength = 20
	nonceLength   = 10
	maxIteration  = 10e5
)

// address for indices and nodes
type doogleAddress []byte

type doogleAddressStr string

func newAddress(host, port, pk string, difficulty int) (doogleAddress, string, error) {
	h := sha1.New()

	var nonce = ""
	var bStr = host + port + pk
	var ret = h.Sum([]byte(bStr))
	var isValid bool = false

	// solve cryptographic puzzle
	for i := 0; i < maxIteration; i++ {
		bNonce, err := getNonce()
		if err != nil {
			continue
		}

		sol := h.Sum(append(ret, bNonce...))
		var count int
		for j := 0; j < difficulty; j++ {
			if sol[j] != 0 {
				break
			} else {
				count++
			}
		}

		if count == difficulty {
			isValid = true
			nonce = string(bNonce)
			break
		}
	}

	if isValid {
		return ret, nonce, nil
	}

	return nil, "", errors.Errorf("could not solve puzzle")
}

func (da doogleAddress) isValid() bool {
	return len(da) == addressLength
}

func getNonce() ([]byte, error) {
	b := make([]byte, nonceLength)
	_, err := rand.Read(b)
	return b, err
}

func (da doogleAddress) xor(a doogleAddress) (doogleAddress, error) {
	if a.isValid() {
		return nil, errors.Errorf("invalid address: ")
	}

	ret := make([]byte, addressLength)
	for i := 0; i < addressLength; i++ {
		ret[i] = da[i] ^ a[i]
	}
	return ret, nil
}

func (da doogleAddress) getHashDistance(a doogleAddress) (doogleAddressStr, error) {
	x, err := da.xor(a)
	if err != nil {
		return "", errors.Wrap(err, "xor computation failed")
	}

	return doogleAddressStr(x), nil
}
