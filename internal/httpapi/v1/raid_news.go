package v1

import (
    "database/sql"
    "errors"
    "net/http"
    "strings"
    "time"
)

type raidNews struct {
    ID int64 `json:"id" bun:"id"`
    Title string `json:"title" bun:"title"`
    Body string `json:"body" bun:"body"`
    Important bool `json:"important" bun:"important"`
    Published bool `json:"published" bun:"published"`
    Acknowledged bool `json:"acknowledged" bun:"acknowledged"`
    CreatedAt string `json:"created_at" bun:"created_at"`
}

type raidNewsInput struct {
    Title string `json:"title"`
    Body string `json:"body"`
    Important bool `json:"important"`
    Published bool `json:"published"`
}

func (a *API) listRaidNews(w http.ResponseWriter, r *http.Request) {
    actor, _ := ActorID(r)
    rows := make([]raidNews, 0)
    err := a.DB.NewRaw(`SELECT n.id, n.title, n.body, n.important, n.published, n.created_at,
        EXISTS(SELECT 1 FROM raid_news_acknowledgements ack WHERE ack.news_id=n.id AND ack.player_id=?) AS acknowledged
        FROM raid_news n WHERE n.published=1 ORDER BY n.created_at DESC, n.id DESC LIMIT 50`, actor).Scan(r.Context(), &rows)
    if err != nil { WriteError(w, 500, "internal", "could not load raid news"); return }
    writeListJSON(w, rows)
}

func (a *API) manageRaidNews(w http.ResponseWriter, r *http.Request) {
    rows := make([]raidNews, 0)
    err := a.DB.NewRaw(`SELECT id,title,body,important,published,created_at,0 AS acknowledged FROM raid_news ORDER BY id DESC LIMIT 100`).Scan(r.Context(), &rows)
    if err != nil { WriteError(w, 500, "internal", "could not load raid news"); return }
    writeListJSON(w, rows)
}

func validateRaidNews(req *raidNewsInput) bool {
    req.Title = strings.TrimSpace(req.Title)
    req.Body = strings.TrimSpace(req.Body)
    return len(req.Title)>0 && len(req.Title)<=120 && len(req.Body)>0 && len(req.Body)<=4000
}

func (a *API) createRaidNews(w http.ResponseWriter, r *http.Request) {
    actor, ok := ActorID(r)
    if !ok { WriteError(w, 401, "unauthenticated", "login required"); return }
    var req raidNewsInput
    if DecodeJSON(r, &req)!=nil || !validateRaidNews(&req) { BadRequest(w, "title or message invalid"); return }
    result, err := a.DB.NewRaw(`INSERT INTO raid_news (title,body,important,published,created_by,created_at,updated_at) VALUES (?,?,?,?,?,?,?)`,
      req.Title,req.Body,req.Important,req.Published,actor,time.Now().UTC(),time.Now().UTC()).Exec(r.Context())
    if err != nil { WriteError(w,500,"internal","could not create raid news"); return }
    id, err := result.LastInsertId()
    if err != nil { WriteError(w,500,"internal","could not read created raid news"); return }
    WriteJSON(w,201,map[string]any{"data":map[string]any{"id":id}})
}

func (a *API) updateRaidNews(w http.ResponseWriter, r *http.Request) {
    id, ok := pathID(r,"id")
    if !ok { BadRequest(w,"invalid news id"); return }
    var req raidNewsInput
    if DecodeJSON(r,&req)!=nil || !validateRaidNews(&req) { BadRequest(w,"title or message invalid"); return }
    result,err:=a.DB.NewRaw(`UPDATE raid_news SET title=?,body=?,important=?,published=?,updated_at=? WHERE id=?`,
       req.Title,req.Body,req.Important,req.Published,time.Now().UTC(),id).Exec(r.Context())
    if err != nil { WriteError(w,500,"internal","could not update raid news"); return }
    affected,err:=result.RowsAffected()
    if err != nil { WriteError(w,500,"internal","could not update raid news"); return }
    if affected==0 { WriteError(w,404,"not_found","raid news not found"); return }
    WriteJSON(w,200,map[string]any{"data":map[string]any{"id":id}})
}

func (a *API) acknowledgeRaidNews(w http.ResponseWriter, r *http.Request) {
    actor,ok:=ActorID(r)
    if !ok { WriteError(w,401,"unauthenticated","login required"); return }
    id,ok:=pathID(r,"id")
    if !ok { BadRequest(w,"invalid news id"); return }
    var exists int
    err:=a.DB.NewRaw(`SELECT 1 FROM raid_news WHERE id=? AND published=1 AND important=1`,id).Scan(r.Context(),&exists)
    if errors.Is(err,sql.ErrNoRows) { WriteError(w,404,"not_found","raid news not found"); return }
    if err!=nil { WriteError(w,500,"internal","could not load raid news"); return }
    _,err=a.DB.NewRaw(`INSERT OR IGNORE INTO raid_news_acknowledgements(news_id,player_id) VALUES (?,?)`,id,actor).Exec(r.Context())
    if err!=nil { WriteError(w,500,"internal","could not acknowledge raid news"); return }
    w.WriteHeader(http.StatusNoContent)
}
