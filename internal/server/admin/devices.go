package admin

import (
	"net/http"
	"strconv"

	"github.com/zxor-org/OronBox-Server/internal/creator"
	"github.com/zxor-org/OronBox-Server/internal/httpx"
	"github.com/zxor-org/OronBox-Server/internal/store"
)

func (a *Handler) HandleAdminAPIXiaomiCloudDevices(w http.ResponseWriter, r *http.Request) {
	devices, err := creator.FetchXiaomiCloudDevices(r.Context())
	if err != nil {
		httpx.WriteJSON(w, http.StatusInternalServerError, httpx.ErrorBody("xiaomi_cloud_failed", err.Error()))
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": devices})
}

func (a *Handler) HandleAdminAPIDeviceModelsList(w http.ResponseWriter, r *http.Request) {
	items, err := a.store.AdminDeviceModels(r.Context())
	if err != nil {
		httpx.WriteJSON(w, http.StatusInternalServerError, httpx.ErrorBody("device_models_failed", err.Error()))
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (a *Handler) HandleAdminAPIDeviceModelsImport(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Items []store.DeviceModel `json:"items"`
	}
	if err := httpx.ReadJSON(r, &body); err != nil {
		httpx.WriteJSON(w, http.StatusBadRequest, httpx.ErrorBody("invalid_request", "invalid json"))
		return
	}
	if len(body.Items) == 0 {
		httpx.WriteJSON(w, http.StatusBadRequest, httpx.ErrorBody("invalid_request", "items cannot be empty"))
		return
	}
	for _, item := range body.Items {
		if item.PdID <= 0 || item.Model == "" || item.Product == "" || item.ID == "" || item.Codename == "" {
			httpx.WriteJSON(w, http.StatusBadRequest, httpx.ErrorBody("invalid_request", "all items must have pd_id, model, product, id, codename"))
			return
		}
	}
	if err := a.store.ImportDeviceModels(r.Context(), body.Items); err != nil {
		a.audit(r, "device_models.import", "failure", err.Error())
		httpx.WriteJSON(w, http.StatusInternalServerError, httpx.ErrorBody("import_failed", err.Error()))
		return
	}
	a.audit(r, "device_models.import", "success", "count="+strconv.Itoa(len(body.Items)))
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true, "imported": len(body.Items)})
}
