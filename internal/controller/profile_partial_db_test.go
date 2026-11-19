package controller

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/shoonya0/ECHO/internal/constants"
	"github.com/shoonya0/ECHO/internal/database"
	"github.com/shoonya0/ECHO/internal/testutil"
)

// PUT /profile changes only the fields the request sends (#32 review). It
// used to rebuild every settings group from the struct, writing "" and false
// for the fields left out: saving one notification setting reset the privacy
// settings, exposing a hidden user to everyone.
func TestUpdateProfileKeepsFieldsNotSent(t *testing.T) {
	ctx := testutil.UseTestDB(t)
	gin.SetMode(gin.TestMode)
	users := database.DB.Collection(string(constants.UserColl))
	id := bson.NewObjectID()
	if _, err := users.InsertOne(ctx, bson.M{"_id": id, "username": "private",
		"presence": bson.M{"status": "dnd", "deviceInfo": "phone"},
		"settings": bson.M{
			"theme":         "dark",
			"notifications": bson.M{"pushEnabled": false, "soundEnabled": true},
			"privacy":       bson.M{"showOnlineStatus": "none", "showLastSeen": true, "allowContactBy": "none"},
		}}); err != nil {
		t.Fatalf("insert: %v", err)
	}

	for _, body := range []string{
		`{"settings":{"notifications":{"pushEnabled":true}}}`,
		`{"settings":{"privacy":{"showLastSeen":false}}}`,
		`{"presence":{"deviceInfo":"tablet"}}`,
	} {
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Request = httptest.NewRequest(http.MethodPut, "/profile/", strings.NewReader(body))
		c.Request.Header.Set("Content-Type", "application/json")
		c.Set("user", struct{}{})
		c.Set("userId", id.Hex())
		UpdateProfile(c)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: code %d; body %s", body, rec.Code, rec.Body)
		}
	}

	var doc struct {
		Presence struct {
			Status     string `bson:"status"`
			DeviceInfo string `bson:"deviceInfo"`
		} `bson:"presence"`
		Settings struct {
			Theme         string `bson:"theme"`
			Notifications struct {
				PushEnabled  bool `bson:"pushEnabled"`
				SoundEnabled bool `bson:"soundEnabled"`
			} `bson:"notifications"`
			Privacy struct {
				ShowOnlineStatus string `bson:"showOnlineStatus"`
				ShowLastSeen     bool   `bson:"showLastSeen"`
				AllowContactBy   string `bson:"allowContactBy"`
			} `bson:"privacy"`
		} `bson:"settings"`
	}
	if err := users.FindOne(ctx, bson.M{"_id": id}).Decode(&doc); err != nil {
		t.Fatalf("read: %v", err)
	}
	checks := []struct {
		field     string
		got, want any
	}{
		{"privacy.showOnlineStatus", doc.Settings.Privacy.ShowOnlineStatus, "none"},
		{"privacy.allowContactBy", doc.Settings.Privacy.AllowContactBy, "none"},
		{"privacy.showLastSeen (sent)", doc.Settings.Privacy.ShowLastSeen, false},
		{"notifications.pushEnabled (sent)", doc.Settings.Notifications.PushEnabled, true},
		{"notifications.soundEnabled", doc.Settings.Notifications.SoundEnabled, true},
		{"theme", doc.Settings.Theme, "dark"},
		{"presence.status", doc.Presence.Status, "dnd"},
		{"presence.deviceInfo (sent)", doc.Presence.DeviceInfo, "tablet"},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s = %v, want %v", c.field, c.got, c.want)
		}
	}
}
