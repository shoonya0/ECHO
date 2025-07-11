package models

type Report struct {
	ReportId       string `json:"report_id" bson:"report_id"`
	ReportedUserId string `json:"reported_user_id" bson:"reported_user_id"`
	ReportedBy     string `json:"reported_by" bson:"reported_by"`
	Reason         string `json:"reason" bson:"reason"`
	CreatedAt      string `json:"created_at" bson:"created_at"`
}
