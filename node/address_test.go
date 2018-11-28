package node

import (
	"fmt"
	"testing"

	"gotest.tools/assert"
)

func TestNewAddress(t *testing.T) {
	cases := []struct {
		host       string
		port       string
		difficulty int
	}{
		{"host1", "port1", 1},
		{"host2", "port2", 2},
	}

	for i, cc := range cases {
		c := cc
		t.Run(fmt.Sprintf("%d-th case", i), func(t *testing.T) {
			na, nonce, err := newNodeAddress(c.host, c.port, nil, c.difficulty)
			assert.Equal(t, nil, err)
			actual := verifyAddress(na, c.host, c.port, nil, nonce, c.difficulty)
			assert.Equal(t, true, actual)
		})
	}
}

func TestVerifyAddress(t *testing.T) {
	cases := []struct {
		da         doogleAddress
		host       string
		port       string
		pk         []byte
		nonce      []byte
		difficulty int
		expected   bool
	}{
		{
			doogleAddress{},
			"",
			"",
			nil,
			nil,
			10,
			false,
		},
		{
			doogleAddress{137, 247, 252, 74, 101, 232, 49, 193, 122, 237, 123, 84, 199, 94, 78, 176, 92, 104, 69, 253},
			"ab",
			"80",
			[]byte("pk"),
			[]byte{124, 101, 169, 225, 58, 47, 235, 38, 179, 1},
			1,
			true,
		},
		{
			doogleAddress{137, 247, 252, 74, 101, 232, 49, 193, 122, 237, 123, 84, 199, 94, 78, 176, 92, 104, 69, 253},
			"ab",
			"80",
			[]byte("pk"),
			[]byte{172, 171, 254, 98, 171, 6, 169, 186, 105, 145},
			2,
			true,
		},
	}

	for i, cc := range cases {
		c := cc
		t.Run(fmt.Sprintf("%d-th case", i), func(t *testing.T) {
			actual := verifyAddress(c.da, c.host, c.port, c.pk, c.nonce, c.difficulty)
			assert.Equal(t, actual, c.expected)
		})
	}
}

func TestGetNonce(t *testing.T) {
	for i := 0; i < 10e3; i++ {
		_, err := getNonce()
		assert.Equal(t, nil, err)
	}
}

func TestLessEqualThan(t *testing.T) {
	cases := []struct {
		da       doogleAddress
		a        doogleAddress
		expected bool
	}{
		{
			doogleAddress{0, 0, 1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0, 0},
			doogleAddress{1, 0, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0, 0, 0},
			true,
		},
		{
			doogleAddress{0, 0, 1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0, 0},
			doogleAddress{0, 0, 0, 0, 1, 0, 1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0, 0, 0},
			false,
		},
		{
			doogleAddress{0, 0, 1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0, 0},
			doogleAddress{0, 0, 1, 0, 1, 0, 1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0, 0, 0},
			true,
		},
	}
	for i, cc := range cases {
		c := cc
		t.Run(fmt.Sprintf("%d-th case", i), func(t *testing.T) {
			actual := c.da.lessThanEqual(c.a)
			assert.Equal(t, actual, c.expected)
		})
	}
}

func TestGetMostSignificantBit(t *testing.T) {
	for i, cc := range []struct {
		input    doogleAddress
		expected int
	}{
		{
			doogleAddress{10, 0, 0, 0, 0, 10, 0, 0, 0, 0, 10, 0, 0, 0, 0, 10, 0, 0, 0, 0},
			155, // 10 = 0b00001010
		},
		{
			doogleAddress{0, 255, 0, 0, 0, 10, 0, 0, 0, 0, 10, 0, 0, 0, 0, 10, 0, 0, 0, 0},
			151, // 255 = 0b11111111
		},
		{
			doogleAddress{255, 255, 0, 0, 0, 10, 0, 0, 0, 0, 10, 0, 0, 0, 0, 10, 0, 0, 0, 0},
			159, // 255 = 0b11111111
		},
	} {
		c := cc
		t.Run(fmt.Sprintf("%d-th case", i), func(t *testing.T) {
			actual := getMostSignificantBit(c.input)
			assert.Equal(t, c.expected, actual)
		})
	}
}
