package services

// MaxPage is the last page the paged lists serve (contacts, user suggestions).
// It keeps the skip far from overflowing, and at 100 per page it is past any
// list a user document holds (#39, #131).
const MaxPage = 10000
