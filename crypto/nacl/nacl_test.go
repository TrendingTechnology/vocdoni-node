package nacl

import (
	"encoding/base64"
	"fmt"
	"testing"
)

func b64dec(in string) []byte {
	b, err := base64.StdEncoding.DecodeString(in)
	if err != nil {
		panic(err)
	}
	return b
}

const (
	jsPub  = "6876524df21d6983724a2b032e41471cc9f1772a9418c4d701fcebb6c306af50"
	jsPriv = "91f86dd7a9ac258c4908ca8fbdd3157f84d1f74ffffcb9fa428fba14a1d40150"
)

var inputs = []struct {
	name     string
	message  []byte
	jsCipher []byte
}{
	{
		name:     "Hello",
		message:  []byte("hello"),
		jsCipher: b64dec("oGwQFMUzXgQ6etTz2UT7Q9ZLJgNMOAjPoX3UicN07gY17mvdUuKhj9RGL0iw8z85Cttj3h4="),
	},
	{
		name:     "Empty",
		message:  []byte(""),
		jsCipher: b64dec("u0z0z7n1c30KHZ7ruB5JDl0CUMKwK8SlR8d/tWBtfQID1k9XOkETSN/0G7/H1ezX"),
	},
	{
		name:     "Symbols",
		message:  []byte("!·$%&/)1234567890"),
		jsCipher: b64dec("qRd094S8AjjE+Z+ZrAhzMyLYZNhxkZlJJReOc4zlzSf+R+wx13xixeJEwKg8nbLn5UZBPTEn81SyFQ8fvHwNidld"),
	},
	{
		name:     "Accents",
		message:  []byte("UTF-8-charsàèìòù"),
		jsCipher: b64dec("Mxf7XYKE3VQa/mH3nvKg/tnhj4UntbfsZ6bbkxXe7wZ2/45I9zKAjhSfsolp31GDDSOiSTMud8gSJYkivUneO3RMpbcG"),
	},
	{
		name:     "Emojis",
		message:  []byte("😃🌟🌹⚖️🚀"),
		jsCipher: b64dec("rWC6RjIqHtdRvjUIsLAyADZX6MhIahEBOnFq8wUV+iZoF1AzFxzUvlFdHMymVVfe7Ls52jBWhxqvHy7YmUatjdend6BQUQ=="),
	},
}

func TestDecryptCiphersFromJS(t *testing.T) {
	keys, err := FromHex(jsPriv)
	if err != nil {
		t.Fatal(err)
	}
	if jsPub != fmt.Sprintf("%x", keys.Public[:]) {
		t.Fatalf("wrong public key derivated from priv key: %s != %x", jsPub, keys.Public[:])
	}

	for _, test := range inputs {
		test := test
		t.Run(test.name, func(t *testing.T) {
			got, err := keys.Decrypt(test.jsCipher)
			if err != nil {
				t.Fatalf("Decrypt error: %v", err)
			}
			if want := test.message; string(got) != string(want) {
				t.Fatalf("Decrypt got %q, want %q", got, want)
			}
		})
	}
}

func TestGenerateEncryptDecrypt(t *testing.T) {
	keys, err := Generate(nil)
	if err != nil {
		t.Fatal(err)
	}
	otherKeys, err := Generate(nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range inputs {
		test := test
		t.Run(test.name, func(t *testing.T) {
			cipher, err := keys.Encrypt(test.message)
			if err != nil {
				t.Fatalf("Encrypt error: %v", err)
			}
			if _, err := otherKeys.Decrypt(cipher); err == nil {
				t.Fatalf("Decrypt with different keys should error")
			}
			got, err := keys.Decrypt(cipher)
			if err != nil {
				t.Fatalf("Decrypt error: %v", err)
			}
			if want := test.message; string(got) != string(want) {
				t.Fatalf("Decrypt got %q, want %q", got, want)
			}
		})
	}
}
