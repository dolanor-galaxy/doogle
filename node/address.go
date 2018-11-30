package node

import (
	"crypto/rand"
	"crypto/sha1"

	"github.com/pkg/errors"
)

const (
	addressLength = 20
	addressBits   = 160
	nonceLength   = 10
	maxIteration  = 10e8
)

// address for indices and nodes
type doogleAddress [addressLength]byte

type doogleAddressStr string

func newNodeAddress(nAddr string, pk []byte, difficulty int) (doogleAddress, []byte, error) {
	var ret = sha1.Sum(append([]byte(nAddr), pk...))

	// solve cryptographic puzzle
	var err error
	var nonce []byte
	var isValid = false
	for i := 0; i < maxIteration; i++ {
		nonce, err = getNonce()
		if err != nil {
			continue
		}

		sol := sha1.Sum(append(ret[:], nonce...))
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

func verifyAddress(da doogleAddress, nAddr string, pk, nonce []byte, difficulty int) bool {

	actual := sha1.Sum(append([]byte(nAddr), pk...))
	if da != actual {
		return false
	}

	sol := sha1.Sum(append(actual[:], nonce...))

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

func getMostSignificantBit(input doogleAddress) int {
	var ret = addressBits - 1
	for i := 0; i < addressLength; i++ {
		var b = input[i]

		if b == 0 {
			ret -= 8
			continue
		}

		var s = 8
		for b != 0 {
			s--
			b = b >> 1
		}
		ret -= s
		break
	}
	return ret
}
