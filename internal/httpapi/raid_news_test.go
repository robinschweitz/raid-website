package httpapi_test

import (
 "encoding/json"
 "fmt"
 "net/http"
 "testing"

 "raid-website/internal/validation"
)

func TestRaidNewsPermissionsAndAcknowledgement(t *testing.T) {
 srv, db := setupServer(t)
 seedAccount(t, db, "news-staff", "password", validation.RoleRaidassistent)
 seedAccount(t, db, "news-member", "password", validation.RoleMember)
 staff := login(t, srv.Client(), srv, "news-staff", "password")
 member := login(t, srv.Client(), srv, "news-member", "password")
 url := srv.URL + "/api/v1/raid-news"
 payload := map[string]any{"title":"Rotation","body":"Alle kommen mal dran.","important":true,"published":true}
 for _, token := range []string{"", member} {
  status, body := do(t, srv.Client(), http.MethodPost, url, token, payload)
  if status != http.StatusForbidden && status != http.StatusUnauthorized { t.Fatalf("unauthorized create: %d %s", status, body) }
 }
 status, body := do(t, srv.Client(), http.MethodPost, url, staff, payload)
 if status != http.StatusCreated { t.Fatalf("create: %d %s", status, body) }
 var created struct { Data struct { ID int64 `json:"id"` } `json:"data"` }
 if err:=json.Unmarshal(body,&created);err!=nil || created.Data.ID==0 { t.Fatalf("invalid create: %s %v",body,err) }
 id := created.Data.ID
 for _, token := range []string{"",member} {
  status, body = do(t,srv.Client(),http.MethodPut,fmt.Sprintf("%s/%d",url,id),token,payload)
  if status != http.StatusForbidden && status != http.StatusUnauthorized { t.Fatalf("unauthorized update: %d %s",status,body) }
 }
 for _, token := range []string{member,staff} {
  status,body=do(t,srv.Client(),http.MethodGet,url,token,nil)
  if status!=http.StatusOK || !json.Valid(body) { t.Fatalf("list: %d %s",status,body) }
 }
 status,body=do(t,srv.Client(),http.MethodPost,fmt.Sprintf("%s/%d/acknowledge",url,id),member,nil)
 if status!=http.StatusNoContent { t.Fatalf("acknowledge: %d %s",status,body) }
 status,body=do(t,srv.Client(),http.MethodPost,fmt.Sprintf("%s/%d/acknowledge",url,id),member,nil)
 if status!=http.StatusNoContent { t.Fatalf("re-acknowledge: %d %s",status,body) }
 status,body=do(t,srv.Client(),http.MethodGet,url,member,nil)
 if status!=http.StatusOK || !json.Valid(body) { t.Fatalf("list after ack: %d %s",status,body) }
 var listed struct { Data []struct { ID int64 `json:"id"`; Acknowledged bool `json:"acknowledged"` } `json:"data"` }
 if err:=json.Unmarshal(body,&listed);err!=nil { t.Fatal(err) }
 found:=false
 for _,n:=range listed.Data { if n.ID==id { found=true; if !n.Acknowledged {t.Fatal("member acknowledgement missing")} } }
 if !found { t.Fatal("news not returned") }
 status,body=do(t,srv.Client(),http.MethodGet,url,staff,nil)
 if status!=http.StatusOK { t.Fatalf("staff list: %d %s",status,body) }
 if err:=json.Unmarshal(body,&listed);err!=nil { t.Fatal(err) }
 for _,n:=range listed.Data {if n.ID==id && n.Acknowledged {t.Fatal("acknowledgement leaked across players")}}
 payload["published"]=false
 status,body=do(t,srv.Client(),http.MethodPut,fmt.Sprintf("%s/%d",url,id),staff,payload)
 if status!=http.StatusOK {t.Fatalf("unpublish: %d %s",status,body)}
 status,body=do(t,srv.Client(),http.MethodPost,fmt.Sprintf("%s/%d/acknowledge",url,id),member,nil)
 if status!=http.StatusNotFound {t.Fatalf("draft acknowledged: %d %s",status,body)}
 status,body=do(t,srv.Client(),http.MethodGet,url,member,nil)
 if status!=http.StatusOK {t.Fatalf("list hidden: %d %s",status,body)}
 if err:=json.Unmarshal(body,&listed);err!=nil {t.Fatal(err)}
 for _,n:=range listed.Data {if n.ID==id {t.Fatal("draft publicly visible")}}
}
