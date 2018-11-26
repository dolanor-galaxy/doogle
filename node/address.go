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

var h = sha1.New()

// address for indices and nodes
type doogleAddress []byte

type doogleAddressStr string

func newAddress(host, port string, pk []byte, difficulty int) (doogleAddress, []byte, error) {

	var nonce []byte
	var err error
	var bStr = host + port
	var ret = h.Sum(append([]byte(bStr), pk...))
	var isValid bool = false

	// solve cryptographic puzzle
	for i := 0; i < maxIteration; i++ {
		nonce, err = getNonce()
		if err != nil {
			continue
		}

		sol := h.Sum(append(ret, nonce...))
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

	return nil, nil, errors.Errorf("could not solve puzzle")
}

func (da doogleAddress) isValid() bool {
	return len(da) == addressLength
}

func verifyAddress(a doogleAddress, host, port string, pk, nonce []byte, difficulty int) bool {
	actual := h.Sum(append([]byte(host+port), pk...))
	if string(a) != string(actual) {
		return false
	}
	sol := h.Sum(append(actual, nonce...))
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

func (da doogleAddress) lessThan(a doogleAddress) bool {
	for i := addressLength - 1; i >= 0; i-- {
		if da[i] != a[i] {
			return da[i] < a[i]
		}
	}
	return true
}
