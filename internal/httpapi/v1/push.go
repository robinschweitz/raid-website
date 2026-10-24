package v1

import (
    "encoding/json"
    "net/http"
    "net/url"
    "strings"
)

// Browser subscriptions are stored for future encrypted VAPID delivery.
// Never accept an arbitrary server URL for network requests without validation.
type pushSubscription struct {
    Endpoint string `json:"endpoint"`
    Keys struct {
        P256DH string `json:"p256dh"`
        Auth string `json:"auth"`
    } `json:"keys"`
}

func permittedPushEndpoint(raw string) bool {
    if len(raw) > 2048 { return false }
    u, err := url.Parse(raw)
    if err != nil || u.Scheme != "https" || u.User != nil || u.Port() != "" ||
        u.RawQuery != "" || u.Fragment != "" { return false }
    host := strings.ToLower(u.Hostname())
    for _, suffix := range []string{"push.services.mozilla.com", "fcm.googleapis.com", "updates.push.services.mozilla.com", "web.push.apple.com"} {
        if host == suffix { return true }
    }
    // Chromium also uses regional push subdomains.
    return strings.HasSuffix(host, ".push.apple.com")
}

func (a *API) registerPush(w http.ResponseWriter, r *http.Request) {
    playerID, ok := ActorID(r)
    if !ok { WriteError(w, http.StatusUnauthorized, "unauthenticated", "login required"); return }
    r.Body = http.MaxBytesReader(w, r.Body, 8192)
    var sub pushSubscription
    if err := json.NewDecoder(r.Body).Decode(&sub); err != nil {
        BadRequest(w, "invalid subscription"); return
    }
    if !permittedPushEndpoint(sub.Endpoint) ||
        len(sub.Keys.P256DH) < 40 || len(sub.Keys.P256DH) > 128 ||
        len(sub.Keys.Auth) < 12 || len(sub.Keys.Auth) > 64 {
        BadRequest(w, "invalid subscription"); return
    }
    // Existing subscription of another player must never be reassigned.
    _, err := a.DB.NewInsert().TableExpr("push_subscriptions").
        ColumnExpr("player_id, endpoint, p256dh, auth").
        Value("?", playerID).Value("?", sub.Endpoint).
        Value("?", sub.Keys.P256DH).Value("?", sub.Keys.Auth).
        Exec(r.Context())
    if err != nil {
        WriteError(w, http.StatusConflict, "subscription_conflict", "subscription unavailable")
        return
    }
    WriteJSON(w, http.StatusCreated, map[string]any{"data": map[string]bool{"enabled": true}})
}

func (a *API) unregisterPush(w http.ResponseWriter, r *http.Request) {
    playerID, ok := ActorID(r)
    if !ok { WriteError(w, http.StatusUnauthorized, "unauthenticated", "login required"); return }
    r.Body = http.MaxBytesReader(w, r.Body, 8192)
    var req struct { Endpoint string `json:"endpoint"` }
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil || len(req.Endpoint) > 2048 {
        BadRequest(w, "invalid subscription"); return
    }
    _, err := a.DB.NewDelete().TableExpr("push_subscriptions").
        Where("player_id = ? AND endpoint = ?", playerID, req.Endpoint).Exec(r.Context())
    if err != nil { WriteError(w, http.StatusInternalServerError, "internal", "could not unregister push"); return }
    w.WriteHeader(http.StatusNoContent)
}
