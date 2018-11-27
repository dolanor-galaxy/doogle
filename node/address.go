package node

import (
	"crypto/rand"
	"crypto/sha1"

	"github.com/pkg/errors"
)

const (
	addressLength = 20
	nonceLength   = 10
	maxIteration  = 10e8
)

// address for indices and nodes
type doogleAddress [addressLength]byte

type doogleAddressStr string

func newNodeAddress(host, port string, pk []byte, difficulty int) (doogleAddress, []byte, error) {
	var bStr = host + port
	var ret = sha1.Sum(append([]byte(bStr), pk...))

	// solve cryptographic puzzle
	var err error
	var nonce []byte
	var isValid = false
	for i := 0; i < maxIteration; i++ {
		nonce, err = getNonce()
		if err != nil {
			continue
		}

		sol := sha1.Sum(append(ret[:addressLength], nonce...))
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
			break
		}
	}

	if isValid {
		return ret, nonce, nil
	}

	return doogleAddress{}, nil, errors.Errorf("could not solve puzzle")
}

func verifyAddress(da doogleAddress, host, port string, pk, nonce []byte, difficulty int) bool {
	actual := sha1.Sum(append([]byte(host+port), pk...))
	if da != actual {
		return false
	}

	sol := sha1.Sum(append(actual[:addressLength], nonce...))
	for i := 0; i < int(difficulty); i++ {
		if sol[i] != 0 {
			return false
		}
	}
	return true
}

func getNonce() ([]byte, error) {
	b := make([]byte, nonceLength)
	_, err := rand.Read(b)
	return b, err
}

func (da doogleAddress) xor(a doogleAddress) doogleAddress {
	ret := doogleAddress{}
	for i := 0; i < addressLength; i++ {
		ret[i] = da[i] ^ a[i]
	}
	return ret
}

func (da doogleAddress) lessThanEqual(a doogleAddress) bool {
	for i := 0; i < addressLength; i++ {
		if da[i] != a[i] {
			return da[i] < a[i]
		}
	}
	return true
}
