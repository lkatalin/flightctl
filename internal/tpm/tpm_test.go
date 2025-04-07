package tpm

import (
	"os"
	"crypto/sha1"
        "testing"

        "github.com/google/go-tpm/tpmutil"
	"github.com/google/go-tpm/legacy/tpm2"
)

func getAuth(name string) tpm2.Digest {
        var auth Digest
        authInput := os.Getenv(name)
        if authInput != "" {
                aa := sha1.Sum([]byte(authInput))
                copy(auth[:], aa[:])
        }
        return auth
}

func TestQuote(t *testing.T) {
        tpm, err := OpenTPM("/dev/tpmrm0")
        if err != nil {
                t.Fatal("Could not open tpm")
        }
        defer tpm.Close()

        // get key from file

        // use default auth of zeroes
        auth := getAuth("")

        handle, err := LoadKey2(tpm, key, auth[:])
        if err != nil {
                t.Fatal("Could not get tpm handle")
        }
        defer CloseKey(tpm, handle)

        data := []byte("abc")
        pcrNums := []int{17,18}
        aikAuth := getAuth("")

        q, values, err := Quote(tpm, handle, data, pcrNums, aikAuth[:])
        if err != nil {
                t.Fatal("Could not get quote")
        }
}
