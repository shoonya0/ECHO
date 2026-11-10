package middleware

import (
	"crypto/subtle"
	"net/http"

	"github.com/shoonya0/ECHO/internal/config"
	"github.com/shoonya0/ECHO/internal/logger"
	"github.com/shoonya0/ECHO/internal/utils"

	"github.com/gin-gonic/gin"
)

// StatsTokenHeader carries the operator token for GET chats/hub/stats.
const StatsTokenHeader = "X-Stats-Token"

// RequireStatsToken lets a request through only when its X-Stats-Token header
// matches STATS_TOKEN. Hub stats are for operators, not app users (#24), so a
// login alone isn't enough. While STATS_TOKEN is unset the route is off. Every
// refusal is the same 404, so the reply doesn't tell whether stats are on.
func RequireStatsToken(ctx *gin.Context) {
	want := config.Cfg.StatsToken
	got := ctx.GetHeader(StatsTokenHeader)

	if want == "" || subtle.ConstantTimeCompare([]byte(got), []byte(want)) != 1 {
		logger.WithContext(ctx.Request.Context()).
			WithField("stats_enabled", want != "").
			Debug("hub stats refused: missing or wrong stats token")
		utils.ErrorResponse(ctx, http.StatusNotFound, "Not found", nil)
		ctx.Abort()
		return
	}

	ctx.Next()
}
