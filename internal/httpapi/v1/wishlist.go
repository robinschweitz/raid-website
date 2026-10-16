package v1

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"raid-website/internal/validation"
	"raid-website/internal/wishlist"
)

// WishlistHandlers exposes wishlist endpoints.
type WishlistHandlers struct {
	Wishlist *wishlist.Service
}

func (h WishlistHandlers) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/characters/{id}/wishlist", RequireAuth(h.listByCharacter))
	mux.HandleFunc("GET /api/v1/characters/{id}/wishlist/addon-export", RequireAuth(h.addonExport))
	mux.HandleFunc("POST /api/v1/characters/{id}/wishlist/items", RequireRole(validation.RoleMember, h.addItem))
	mux.HandleFunc("PUT /api/v1/characters/{id}/wishlist/items/{itemID}", RequireRole(validation.RoleMember, h.updateItem))
	mux.HandleFunc("DELETE /api/v1/characters/{id}/wishlist/items/{itemID}", RequireRole(validation.RoleMember, h.removeItem))
	mux.HandleFunc("GET /api/v1/raids/{id}/wishlist", RequireRole(validation.RoleRaidassistent, h.raidWishlist))
}

type wishlistAddRequest struct {
	ItemID     string        `json:"item_id"`
	ItemName   string        `json:"item_name"`
	Priority   int           `json:"priority"`
	Notes      string        `json:"notes"`
	Spec       wishlist.Spec `json:"spec"`
	IsEnhanced bool          `json:"is_enhanced"`
}

type wishlistUpdateRequest struct {
	Priority   int           `json:"priority"`
	Notes      string        `json:"notes"`
	Spec       wishlist.Spec `json:"spec"`
	IsEnhanced bool          `json:"is_enhanced"`
}

type wishlistAddonExportResponse struct {
	Text string `json:"text"`
}

// readableCharacter returns the character ID when the actor may read its
// wishlist. Staff may read any wishlist; everybody else only the wishlists of
// their own characters. Ownership is decided by the character, not by list
// contents, so an owner can always read an empty wishlist.
func (h WishlistHandlers) readableCharacter(w http.ResponseWriter, r *http.Request) (int64, bool) {
	charID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || charID <= 0 {
		WriteError(w, http.StatusBadRequest, "bad_request", "Ungültige Charakter-ID")
		return 0, false
	}
	actor, ok := CurrentPlayer(r)
	if !ok {
		WriteError(w, http.StatusUnauthorized, "unauthorized", "Nicht angemeldet")
		return 0, false
	}
	if actor.Role.AtLeast(validation.RoleRaidassistent) {
		return charID, true
	}
	owner, err := h.Wishlist.Owner(r.Context(), charID)
	if err != nil {
		switch {
		case errors.Is(err, wishlist.ErrCharacterNotFound):
			WriteError(w, http.StatusNotFound, "not_found", "Charakter nicht gefunden")
		case errors.Is(err, wishlist.ErrInvalid):
			WriteError(w, http.StatusBadRequest, "bad_request", "Ungültige Anfrage")
		default:
			WriteError(w, http.StatusInternalServerError, "internal", "Wunschliste konnte nicht geladen werden")
		}
		return 0, false
	}
	if owner != actor.ID {
		WriteError(w, http.StatusForbidden, "forbidden", "Keine Berechtigung zum Anzeigen dieser Wunschliste")
		return 0, false
	}
	return charID, true
}

func (h WishlistHandlers) listByCharacter(w http.ResponseWriter, r *http.Request) {
	charID, ok := h.readableCharacter(w, r)
	if !ok {
		return
	}
	items, err := h.Wishlist.ListByCharacter(r.Context(), charID)
	if err != nil {
		if errors.Is(err, wishlist.ErrInvalid) {
			WriteError(w, http.StatusBadRequest, "bad_request", "Ungültige Anfrage")
			return
		}
		WriteError(w, http.StatusInternalServerError, "internal", "Wunschliste konnte nicht geladen werden")
		return
	}
	writeListJSON(w, items)
}

func (h WishlistHandlers) addItem(w http.ResponseWriter, r *http.Request) {
	charID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || charID <= 0 {
		WriteError(w, http.StatusBadRequest, "bad_request", "Ungültige Charakter-ID")
		return
	}
	var req wishlistAddRequest
	if err := DecodeJSON(r, &req); err != nil {
		WriteError(w, http.StatusBadRequest, "bad_request", "Ungültiges JSON")
		return
	}
	actor, ok := CurrentPlayer(r)
	if !ok {
		WriteError(w, http.StatusUnauthorized, "unauthorized", "Nicht angemeldet")
		return
	}
	itemID := strings.TrimSpace(req.ItemID)
	itemName := strings.TrimSpace(req.ItemName)
	if itemID == "" || itemName == "" {
		WriteError(w, http.StatusBadRequest, "bad_request", "item_id und item_name sind erforderlich")
		return
	}
	wish, err := h.Wishlist.Add(r.Context(), actor.ID, charID, itemID, itemName, req.Priority, req.Notes, wishlist.RollChoice{Spec: req.Spec, IsEnhanced: req.IsEnhanced})
	if err != nil {
		switch {
		case errors.Is(err, wishlist.ErrNotOwner):
			WriteError(w, http.StatusForbidden, "forbidden", "Nur eigener Charakter")
			return
		case errors.Is(err, wishlist.ErrCharacterNotFound):
			WriteError(w, http.StatusNotFound, "not_found", "Charakter nicht gefunden")
			return
		case errors.Is(err, wishlist.ErrConflict):
			WriteError(w, http.StatusConflict, "wishlist_conflict", "Bereits auf Wunschliste")
			return
		case errors.Is(err, wishlist.ErrInvalid):
			WriteError(w, http.StatusBadRequest, "bad_request", "Ungültige Anfrage")
			return
		default:
			WriteError(w, http.StatusInternalServerError, "internal", "Wunschliste konnte nicht aktualisiert werden")
			return
		}
	}
	WriteJSON(w, http.StatusCreated, map[string]any{"data": wish})
}

func (h WishlistHandlers) addonExport(w http.ResponseWriter, r *http.Request) {
	charID, ok := h.readableCharacter(w, r)
	if !ok {
		return
	}
	text, err := h.Wishlist.AddonExport(r.Context(), charID, time.Now().UTC())
	if err != nil {
		if errors.Is(err, wishlist.ErrCharacterNotFound) {
			WriteError(w, http.StatusNotFound, "not_found", "Charakter nicht gefunden")
			return
		}
		WriteError(w, http.StatusInternalServerError, "internal", "Wunschliste konnte nicht exportiert werden")
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{"data": wishlistAddonExportResponse{Text: text}})
}

func (h WishlistHandlers) updateItem(w http.ResponseWriter, r *http.Request) {
	charID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || charID <= 0 {
		WriteError(w, http.StatusBadRequest, "bad_request", "Ungültige Charakter-ID")
		return
	}
	itemID := r.PathValue("itemID")
	var req wishlistUpdateRequest
	if err := DecodeJSON(r, &req); err != nil {
		WriteError(w, http.StatusBadRequest, "bad_request", "Ungültiges JSON")
		return
	}
	actor, ok := CurrentPlayer(r)
	if !ok {
		WriteError(w, http.StatusUnauthorized, "unauthorized", "Nicht angemeldet")
		return
	}
	wish, err := h.Wishlist.Update(r.Context(), actor.ID, charID, itemID, req.Priority, req.Notes, wishlist.RollChoice{Spec: req.Spec, IsEnhanced: req.IsEnhanced})
	if err != nil {
		switch {
		case errors.Is(err, wishlist.ErrNotOwner):
			WriteError(w, http.StatusForbidden, "forbidden", "Nur eigener Charakter")
		case errors.Is(err, wishlist.ErrCharacterNotFound):
			WriteError(w, http.StatusNotFound, "not_found", "Charakter nicht gefunden")
		case errors.Is(err, wishlist.ErrNotFound):
			WriteError(w, http.StatusNotFound, "not_found", "Nicht auf Wunschliste gefunden")
		case errors.Is(err, wishlist.ErrInvalid):
			WriteError(w, http.StatusBadRequest, "bad_request", "Spec muss MAIN oder SECOND sein; Enhanced nur für Main-Spec")
		default:
			WriteError(w, http.StatusInternalServerError, "internal", "Wunschliste konnte nicht aktualisiert werden")
		}
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{"data": wish})
}

func (h WishlistHandlers) removeItem(w http.ResponseWriter, r *http.Request) {
	charID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || charID <= 0 {
		WriteError(w, http.StatusBadRequest, "bad_request", "Ungültige Charakter-ID")
		return
	}
	itemID := r.PathValue("itemID")
	if itemID == "" {
		WriteError(w, http.StatusBadRequest, "bad_request", "itemID erforderlich")
		return
	}
	actor, ok := CurrentPlayer(r)
	if !ok {
		WriteError(w, http.StatusUnauthorized, "unauthorized", "Nicht angemeldet")
		return
	}
	if err := h.Wishlist.Remove(r.Context(), actor.ID, charID, itemID); err != nil {
		switch {
		case errors.Is(err, wishlist.ErrNotOwner):
			WriteError(w, http.StatusForbidden, "forbidden", "Nur eigener Charakter")
			return
		case errors.Is(err, wishlist.ErrCharacterNotFound):
			WriteError(w, http.StatusNotFound, "not_found", "Charakter nicht gefunden")
			return
		case errors.Is(err, wishlist.ErrNotFound):
			WriteError(w, http.StatusNotFound, "not_found", "Nicht auf Wunschliste gefunden")
			return
		case errors.Is(err, wishlist.ErrInvalid):
			WriteError(w, http.StatusBadRequest, "bad_request", "Ungültige Anfrage")
			return
		default:
			WriteError(w, http.StatusInternalServerError, "internal", "Wunschliste konnte nicht aktualisiert werden")
			return
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

type raidWishlistItem struct {
	ItemID     string                          `json:"item_id"`
	Characters []raidWishlistCharacterInterest `json:"characters"`
	Count      int                             `json:"count"`
}

type raidWishlistCharacterInterest struct {
	CharacterID   int64  `json:"character_id"`
	CharacterName string `json:"character_name"`
	PlayerID      int64  `json:"player_id"`
	PlayerName    string `json:"player_name"`
}

func (h WishlistHandlers) raidWishlist(w http.ResponseWriter, r *http.Request) {
	raidID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || raidID <= 0 {
		WriteError(w, http.StatusBadRequest, "bad_request", "Ungültige Raid-ID")
		return
	}
	itemID := strings.TrimSpace(r.URL.Query().Get("item_id"))
	if itemID != "" {
		rows, err := h.Wishlist.BulkByItemForRaid(r.Context(), raidID, itemID)
		if err != nil {
			WriteError(w, http.StatusInternalServerError, "internal", "Wunschlisten-Interessen konnten nicht geladen werden")
			return
		}
		out := raidWishlistItem{ItemID: itemID, Characters: make([]raidWishlistCharacterInterest, 0, len(rows))}
		for _, row := range rows {
			out.Characters = append(out.Characters, raidWishlistCharacterInterest{
				CharacterID:   row.CharacterID,
				CharacterName: row.CharacterName,
				PlayerID:      row.PlayerID,
				PlayerName:    row.PlayerName,
			})
		}
		out.Count = len(out.Characters)
		WriteJSON(w, http.StatusOK, map[string]any{"data": out})
		return
	}
	rows, err := h.Wishlist.BulkByItemsForRaid(r.Context(), raidID)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal", "Wunschlisten-Interessen konnten nicht geladen werden")
		return
	}
	m := map[string][]raidWishlistCharacterInterest{}
	for _, row := range rows {
		m[row.ItemID] = append(m[row.ItemID], raidWishlistCharacterInterest{
			CharacterID:   row.CharacterID,
			CharacterName: row.CharacterName,
			PlayerID:      row.PlayerID,
			PlayerName:    row.PlayerName,
		})
	}
	out := make([]raidWishlistItem, 0, len(m))
	for iid, list := range m {
		out = append(out, raidWishlistItem{ItemID: iid, Characters: list, Count: len(list)})
	}
	writeListJSON(w, out)
}
