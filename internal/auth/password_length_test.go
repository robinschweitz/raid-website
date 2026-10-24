package auth
import("errors";"strings";"testing")
func TestPasswordLengthBound(t *testing.T){
 for _,password:=range []string{strings.Repeat("a",1025),strings.Repeat("ä",513)}{
  if err:=ValidatePassword(password);!errors.Is(err,ErrPasswordTooLong){t.Fatalf("wanted length error: %v",err)}
 }
 for _,password:=range []string{strings.Repeat("a",73),strings.Repeat("a",1024),strings.Repeat("ä",36)}{
  if err:=ValidatePassword(password);err!=nil{t.Fatal(err)}
  hash,err:=HashPassword(password);if err!=nil{t.Fatal(err)}
  if err:=verifyStoredPassword(hash,password);err!=nil{t.Fatal(err)}
 }
}
