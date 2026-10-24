package auth
import (
 "crypto/rand"
 "crypto/subtle"
 "encoding/base64"
 "errors"
 "fmt"
 "strings"
 "golang.org/x/crypto/argon2"
 "golang.org/x/crypto/bcrypt"
)
const (
 argonTime uint32 = 3
 argonMemory uint32 = 64*1024
 argonThreads uint8 = 1
 maxPasswordBytes = 1024
)
func HashPassword(password string) (string,error) {
 salt:=make([]byte,16)
 if _,err:=rand.Read(salt);err!=nil{return "",err}
 key:=argon2.IDKey([]byte(password),salt,argonTime,argonMemory,argonThreads,32)
 enc:=base64.RawStdEncoding
 return fmt.Sprintf("$argon2id$v=19$m=%d,t=%d,p=%d$%s$%s",argonMemory,argonTime,argonThreads,enc.EncodeToString(salt),enc.EncodeToString(key)),nil
}
func verifyStoredPassword(hash,password string) error {
 if strings.HasPrefix(hash,"$2a$")||strings.HasPrefix(hash,"$2b$")||strings.HasPrefix(hash,"$2y$"){return bcrypt.CompareHashAndPassword([]byte(hash),[]byte(password))}
 parts:=strings.Split(hash,"$")
 if len(parts)!=6||parts[0]!=""||parts[1]!="argon2id"||parts[2]!="v=19"{return errors.New("invalid password hash")}
 var m,t uint32
 var p uint8
 n,err:=fmt.Sscanf(parts[3],"m=%d,t=%d,p=%d",&m,&t,&p)
 if err!=nil||n!=3||fmt.Sprintf("m=%d,t=%d,p=%d",m,t,p)!=parts[3]||p==0||p>8||m<8*uint32(p)||m>256*1024||t==0||t>10{return errors.New("invalid argon2 parameters")}
 salt,err:=base64.RawStdEncoding.DecodeString(parts[4]);if err!=nil||len(salt)<8||len(salt)>64{return errors.New("invalid argon2 salt")}
 expected,err:=base64.RawStdEncoding.DecodeString(parts[5]);if err!=nil||len(expected)<16||len(expected)>64{return errors.New("invalid argon2 hash")}
 actual:=argon2.IDKey([]byte(password),salt,t,m,p,uint32(len(expected)))
 if subtle.ConstantTimeCompare(actual,expected)!=1{return errors.New("password mismatch")}
 return nil
}
