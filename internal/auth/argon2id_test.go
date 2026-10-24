package auth
import("context";"strings";"testing";"golang.org/x/crypto/bcrypt")
func TestArgon2idRoundTrip(t *testing.T){
 pw:="a-secure-long-password"
 a,err:=HashPassword(pw);if err!=nil{t.Fatal(err)}
 b,err:=HashPassword(pw);if err!=nil{t.Fatal(err)}
 if a==b||!strings.HasPrefix(a,"$argon2id$v=19$"){t.Fatal("invalid salts or PHC format")}
 if err:=verifyStoredPassword(a,pw);err!=nil{t.Fatal(err)}
 if err:=verifyStoredPassword(a,"wrong");err==nil{t.Fatal("accepted wrong password")}
 if err:=verifyStoredPassword(a+"x",pw);err==nil{t.Fatal("accepted corrupt hash")}
}
func TestBcryptMigrationOnLogin(t *testing.T){
 ctx:=context.Background()
 db,svc:=loginLimitService(t)
 old,err:=bcrypt.GenerateFromPassword([]byte("valid-password"),bcrypt.DefaultCost);if err!=nil{t.Fatal(err)}
 if _,err:=db.DB.ExecContext(ctx,"UPDATE players SET password_hash = ? WHERE username = 'owner'",string(old));err!=nil{t.Fatal(err)}
 if result,err:=svc.StartLogin(ctx,"owner","wrong-password",limitedAddress);result!=nil||err==nil{t.Fatal("accepted wrong password")}
 var saved string
 if err:=db.DB.QueryRowContext(ctx,"SELECT password_hash FROM players WHERE username = 'owner'").Scan(&saved);err!=nil{t.Fatal(err)}
 if saved!=string(old){t.Fatal("incorrect login migrated hash")}
 result,err:=svc.StartLogin(ctx,"owner","valid-password",limitedAddress)
 if err!=nil||result==nil||result.Token==""{t.Fatalf("login: %v",err)}
 if err:=db.DB.QueryRowContext(ctx,"SELECT password_hash FROM players WHERE username = 'owner'").Scan(&saved);err!=nil{t.Fatal(err)}
 if !strings.HasPrefix(saved,"$argon2id$"){t.Fatal("bcrypt not migrated")}
}
