// SPDX-License-Identifier: Apache-2.0

package gouncer_test

import (
	"fmt"

	"github.com/google/uuid"

	"github.com/gopherium/gouncer"
)

func ExampleNewUser() {
	u, err := gouncer.NewUser("ada@example.com", "Ada Lovelace", "correct horse battery")
	if err != nil {
		return
	}

	fmt.Println(u.Email)
	// Output: ada@example.com
}

func ExampleVerifyPassword() {
	u, err := gouncer.NewUser("ada@example.com", "Ada Lovelace", "correct horse battery")
	if err != nil {
		return
	}

	fmt.Println(gouncer.VerifyPassword(u.PasswordHash, "correct horse battery"))
	fmt.Println(gouncer.VerifyPassword(u.PasswordHash, "wrong password entirely"))
	// Output:
	// true
	// false
}

func ExampleNewSession() {
	s, err := gouncer.NewSession(uuid.Must(uuid.NewV7()))
	if err != nil {
		return
	}

	_ = s.Token
	fmt.Println(len(s.TokenHash))
	// Output: 32
}
