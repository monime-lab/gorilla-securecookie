// Copyright 2012 The Gorilla Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package securecookie

import (
	"crypto/aes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"reflect"
	"testing"

	fuzz "github.com/google/gofuzz"
)

// Asserts that Error and MultiError are Error implementations.
var _ Error = Error{}

var testCookies = []interface{}{
	map[string]string{"foo": "bar"},
	map[string]string{"baz": "ding"},
}

var testStrings = []string{"foo", "bar", "baz"}

func TestSecureCookie(t *testing.T) {
	// TODO test too old / too new timestamps
	s1 := New(GenerateRandomKey(64), GenerateRandomKey(32))
	s2 := New([]byte("54321"), []byte("6543210987654321"))
	value := map[string]interface{}{
		"foo": "bar",
		"baz": float64(128),
	}

	for i := 0; i < 50; i++ {
		// Running this multiple times to check if any special character
		// breaks encoding/decoding.
		encoded, err1 := s1.Encode("__Secure-sid", value)
		if err1 != nil {
			t.Error(err1)
			continue
		}
		dst := make(map[string]interface{})
		err2 := s1.Decode("__Host-sid", encoded, &dst)
		if err2 != nil {
			t.Fatalf("%v: %v", err2, encoded)
		}
		if !reflect.DeepEqual(dst, value) {
			t.Fatalf("Expected %v, got %v.", value, dst)
		}
		dst2 := make(map[string]interface{})
		err3 := s2.Decode("sid", encoded, &dst2)
		if err3 == nil {
			t.Fatalf("Expected failure decoding.")
		}
		_, ok := err3.(Error)
		if !ok {
			t.Fatalf("Expected error to implement Error, got: %#v", err3)
		}
	}
}

func TestDecodeInvalid(t *testing.T) {
	// List of invalid cookies, which must not be accepted, base64-decoded
	// (they will be encoded before passing to Decode).
	invalidCookies := []string{
		"",
		" ",
		"\n",
		"||",
		"|||",
		"cookie",
	}
	s := New([]byte("12345"), nil)
	var dst string
	for i, v := range invalidCookies {
		for _, enc := range []*base64.Encoding{
			base64.StdEncoding,
			base64.URLEncoding,
		} {
			err := s.Decode("name", enc.EncodeToString([]byte(v)), &dst)
			if err == nil {
				t.Fatalf("%d: expected failure decoding", i)
			}
			_, ok := err.(Error)
			if !ok {
				t.Fatalf("%d: Expected IsDecode(), got: %#v", i, err)
			}
		}
	}
}

func TestAuthentication(t *testing.T) {
	hash := hmac.New(sha256.New, []byte("secret-key"))
	for _, value := range testStrings {
		hash.Reset()
		signed := createMac(hash, []byte(value))
		hash.Reset()
		err := verifyMac(hash, []byte(value), signed)
		if err != nil {
			t.Error(err)
		}
	}
}

func TestEncryption(t *testing.T) {
	block, err := aes.NewCipher([]byte("1234567890123456"))
	if err != nil {
		t.Fatalf("Block could not be created")
	}
	var encrypted, decrypted []byte
	for _, value := range testStrings {
		if encrypted, err = encrypt(block, []byte(value)); err != nil {
			t.Error(err)
		} else {
			if decrypted, err = decrypt(block, encrypted); err != nil {
				t.Error(err)
			}
			if string(decrypted) != value {
				t.Errorf("Expected %v, got %v.", value, string(decrypted))
			}
		}
	}
}

func TestJSONSerialization(t *testing.T) {
	var (
		sz           JSONEncoder
		serialized   []byte
		deserialized map[string]string
		err          error
	)
	for _, value := range testCookies {
		if serialized, err = sz.Serialize(value); err != nil {
			t.Error(err)
		} else {
			deserialized = make(map[string]string)
			if err = sz.Deserialize(serialized, &deserialized); err != nil {
				t.Error(err)
			}
			if fmt.Sprintf("%v", deserialized) != fmt.Sprintf("%v", value) {
				t.Errorf("Expected %v, got %v.", value, deserialized)
			}
		}
	}
}

func TestNopSerialization(t *testing.T) {
	cookieData := "fooobar123"
	sz := NopEncoder{}

	if _, err := sz.Serialize(cookieData); err != errValueNotByte {
		t.Fatal("Expected error passing string")
	}
	dat, err := sz.Serialize([]byte(cookieData))
	if err != nil {
		t.Fatal(err)
	}
	if (string(dat)) != cookieData {
		t.Fatal("Expected serialized data to be same as source")
	}

	var dst []byte
	if err = sz.Deserialize(dat, dst); err != errValueNotBytePtr {
		t.Fatal("Expect error unless you pass a *[]byte")
	}
	if err = sz.Deserialize(dat, &dst); err != nil {
		t.Fatal(err)
	}
	if (string(dst)) != cookieData {
		t.Fatal("Expected deserialized data to be same as source")
	}
}

func TestEncoding(t *testing.T) {
	for _, value := range testStrings {
		encoded := encode([]byte(value))
		decoded, err := decode(encoded)
		if err != nil {
			t.Error(err)
		} else if string(decoded) != value {
			t.Errorf("Expected %v, got %s.", value, string(decoded))
		}
	}
}

func TestMultiNoCodecs(t *testing.T) {
	_, err := EncodeMulti("foo", "bar")
	if err != errNoCodecs {
		t.Errorf("EncodeMulti: bad value for error, got: %v", err)
	}

	var dst []byte
	err = DecodeMulti("foo", "bar", &dst)
	if err != errNoCodecs {
		t.Errorf("DecodeMulti: bad value for error, got: %v", err)
	}
}

// ----------------------------------------------------------------------------

type FooBar struct {
	Foo int
	Bar string
}

func TestCustomType(t *testing.T) {
	s1 := New([]byte("12345"), []byte("1234567890123456"))
	// Type is not registered in gob. (!!!)
	src := &FooBar{42, "bar"}
	encoded, _ := s1.Encode("sid", src)

	dst := &FooBar{}
	_ = s1.Decode("sid", encoded, dst)
	if dst.Foo != 42 || dst.Bar != "bar" {
		t.Fatalf("Expected %#v, got %#v", src, dst)
	}
}

type Cookie struct {
	B bool
	I int
	S string
}

func FuzzEncodeDecode(f *testing.F) {
	fuzzer := fuzz.New()
	s1 := New([]byte("12345"), []byte("1234567890123456"))
	s1.maxLength = 0

	for i := 0; i < 100000; i++ {
		var c Cookie
		fuzzer.Fuzz(&c)
		f.Add(c.B, c.I, c.S)
	}

	f.Fuzz(func(t *testing.T, b bool, i int, s string) {
		c := Cookie{b, i, s}
		encoded, err := s1.Encode("sid", c)
		if err != nil {
			t.Errorf("Encode failed: %v", err)
		}
		dc := Cookie{}
		err = s1.Decode("sid", encoded, &dc)
		if err != nil {
			t.Errorf("Decode failed: %v", err)
		}
		if dc != c {
			t.Fatalf("Expected %v, got %v.", s, dc)
		}
	})
}
