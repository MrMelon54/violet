package fake

import (
	"crypto/rand"
	"crypto/rsa"
	"github.com/MrMelon54/mjwt"
	"github.com/MrMelon54/mjwt/auth"
	"github.com/MrMelon54/mjwt/claims"
	"time"
)

var SnakeOilProv = GenSnakeOilProv()

func GenSnakeOilProv() mjwt.Signer {
	key, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		panic(err)
	}
	return mjwt.NewMJwtSigner("violet.test", key)
}

func GenSnakeOilKey(perm string) string {
	p := claims.NewPermStorage()
	p.Set(perm)
	val, err := SnakeOilProv.GenerateJwt("abc", "abc", nil, 5*time.Minute, auth.AccessTokenClaims{Perms: p})
	if err != nil {
		panic(err)
	}
	return val
}
