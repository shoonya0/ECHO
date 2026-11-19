package services

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/shoonya0/ECHO/internal/constants"
	"github.com/shoonya0/ECHO/internal/database"
	"github.com/shoonya0/ECHO/internal/models"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// lockedBroadcaster records events from concurrent sends.
type lockedBroadcaster struct {
	mu     sync.Mutex
	events []models.WebSocketMessage
}

func (b *lockedBroadcaster) PublishToChat(_ string, message *models.WebSocketMessage) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.events = append(b.events, *message)
	return nil
}

// chatLastSeq reads a chat's stats.lastSeq.
func chatLastSeq(t *testing.T, ctx context.Context, chatID bson.ObjectID) int64 {
	t.Helper()
	var chat models.Chat
	if err := database.DB.Collection(string(constants.ChatColl)).FindOne(ctx, bson.M{"_id": chatID}).Decode(&chat); err != nil {
		t.Fatalf("read chat: %v", err)
	}
	return chat.Stats.LastSeq
}

// #43: sends at the same time get the numbers 1 to n, each once, and the live
// event carries the number, so a client can tell when it missed one.
func TestSendMessageNumbersMessagesWithoutGaps(t *testing.T) {
	ctx := useTestDB(t)
	rec := &lockedBroadcaster{}
	SetBroadcaster(rec)
	t.Cleanup(func() { SetBroadcaster(nil) })

	owner, member := bson.NewObjectID(), bson.NewObjectID()
	insertTestUsers(t, ctx, owner, member)
	chat := newTestGroup(t, ctx, owner, member)

	const sends = 12
	seqs := make([]int64, sends)
	errs := make([]error, sends)
	var wg sync.WaitGroup
	for i := range sends {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sender := owner
			if i%2 == 1 {
				sender = member
			}
			msg, err := SendMessage(ctx, chat.ChatID, sender, "hi", "text", nil, nil)
			errs[i] = err
			if err == nil {
				seqs[i] = msg.Seq
			}
		}()
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("send %d: %v", i, err)
		}
	}

	slices.Sort(seqs)
	for i, seq := range seqs {
		if seq != int64(i+1) {
			t.Fatalf("seqs = %v, want 1 to %d each once", seqs, sends)
		}
	}
	if got := chatLastSeq(t, ctx, chat.ChatID); got != sends {
		t.Fatalf("stats.lastSeq = %d, want %d", got, sends)
	}

	stored, err := database.DB.Collection(string(constants.MessageColl)).Distinct(ctx, "seq", bson.M{"chatId": chat.ChatID}).Raw()
	if err != nil {
		t.Fatalf("distinct seqs: %v", err)
	}
	if n := len(must(stored.Values())); n != sends {
		t.Fatalf("stored %d distinct seqs, want %d", n, sends)
	}

	var live []int64
	for _, e := range rec.events {
		if m, ok := e.Data.(models.ChatMessage); ok {
			live = append(live, m.Seq)
		}
	}
	slices.Sort(live)
	if !slices.Equal(live, seqs) {
		t.Fatalf("live event seqs = %v, want %v", live, seqs)
	}
}

func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}

// #43: catching up returns the messages after a seq, oldest first, in pages, and
// only to members.
func TestGetChatMessagesAfterSeq(t *testing.T) {
	ctx := useTestDB(t)
	SetBroadcaster(&recordingBroadcaster{})
	t.Cleanup(func() { SetBroadcaster(nil) })

	owner, member, outsider := bson.NewObjectID(), bson.NewObjectID(), bson.NewObjectID()
	insertTestUsers(t, ctx, owner, member, outsider)
	chat := newTestGroup(t, ctx, owner, member)
	for range 5 {
		if _, err := SendMessage(ctx, chat.ChatID, owner, "hi", "text", nil, nil); err != nil {
			t.Fatalf("SendMessage: %v", err)
		}
	}

	page, hasMore, err := GetChatMessagesAfterSeq(ctx, member, chat.ChatID, 2, 1)
	if err != nil {
		t.Fatalf("GetChatMessagesAfterSeq: %v", err)
	}
	if got := messageSeqs(page); !slices.Equal(got, []int64{2, 3}) || !hasMore {
		t.Fatalf("first page = %v hasMore=%v, want [2 3] true", got, hasMore)
	}
	page, hasMore, err = GetChatMessagesAfterSeq(ctx, member, chat.ChatID, 2, 3)
	if err != nil {
		t.Fatalf("GetChatMessagesAfterSeq: %v", err)
	}
	if got := messageSeqs(page); !slices.Equal(got, []int64{4, 5}) || hasMore {
		t.Fatalf("second page = %v hasMore=%v, want [4 5] false", got, hasMore)
	}
	page, _, err = GetChatMessagesAfterSeq(ctx, member, chat.ChatID, 10, 5)
	if err != nil || len(page) != 0 {
		t.Fatalf("after the newest = %v, %v; want none", messageSeqs(page), err)
	}

	if _, _, err := GetChatMessagesAfterSeq(ctx, outsider, chat.ChatID, 10, 0); !errors.Is(err, ErrChatNotFound) {
		t.Fatalf("outsider: err = %v, want ErrChatNotFound", err)
	}
}

func messageSeqs(messages []models.Message) []int64 {
	seqs := make([]int64, len(messages))
	for i, m := range messages {
		seqs[i] = m.Seq
	}
	return seqs
}

// #43: a chat from before #43 keeps the numbers 1 to k for its k old messages,
// oldest first, even when someone sends before the backfill runs. A message an older
// server copy saves after that gets a number after the newest. Messages of a deleted
// chat are left alone, and once a final run is done, later runs skip the scan.
func TestBackfillMessageSeqs(t *testing.T) {
	ctx := useTestDB(t)
	SetBroadcaster(&recordingBroadcaster{})
	t.Cleanup(func() { SetBroadcaster(nil) })
	messages := database.DB.Collection(string(constants.MessageColl))

	owner, member := bson.NewObjectID(), bson.NewObjectID()
	insertTestUsers(t, ctx, owner, member)
	chat := newTestGroup(t, ctx, owner, member)
	// A chat from before #43: no stats.lastSeq, three messages without a seq.
	if _, err := database.DB.Collection(string(constants.ChatColl)).UpdateOne(ctx,
		bson.M{"_id": chat.ChatID}, bson.M{"$unset": bson.M{"stats.lastSeq": ""}}); err != nil {
		t.Fatalf("unset lastSeq: %v", err)
	}
	old := make([]bson.ObjectID, 3)
	for i := range old {
		old[i] = bson.NewObjectID()
	}
	goneChat := bson.NewObjectID()
	if _, err := messages.InsertMany(ctx, []any{
		bson.M{"_id": old[2], "chatId": chat.ChatID}, // inserted out of order on purpose
		bson.M{"_id": old[0], "chatId": chat.ChatID},
		bson.M{"_id": old[1], "chatId": chat.ChatID},
		bson.M{"_id": bson.NewObjectID(), "chatId": goneChat},
	}); err != nil {
		t.Fatalf("insert messages: %v", err)
	}

	sent, err := SendMessage(ctx, chat.ChatID, owner, "after the deploy", "text", nil, nil)
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	if sent.Seq != 4 {
		t.Fatalf("first message after #43 has seq %d, want 4 (after the 3 old ones)", sent.Seq)
	}
	// An older server copy saves one more during the deploy.
	late := bson.NewObjectID()
	if _, err := messages.InsertOne(ctx, bson.M{"_id": late, "chatId": chat.ChatID}); err != nil {
		t.Fatalf("insert late message: %v", err)
	}

	for run, want := range []int64{4, 0} {
		n, err := BackfillMessageSeqs(ctx, false)
		if err != nil {
			t.Fatalf("run %d: BackfillMessageSeqs: %v", run+1, err)
		}
		if n != want {
			t.Fatalf("run %d: numbered %d, want %d", run+1, n, want)
		}
	}

	want := map[bson.ObjectID]int64{old[0]: 1, old[1]: 2, old[2]: 3, sent.ID: 4, late: 5}
	for id, seq := range want {
		var m models.Message
		if err := messages.FindOne(ctx, bson.M{"_id": id}).Decode(&m); err != nil {
			t.Fatalf("read message: %v", err)
		}
		if m.Seq != seq {
			t.Fatalf("message %s has seq %d, want %d", id.Hex(), m.Seq, seq)
		}
	}
	if got := chatLastSeq(t, ctx, chat.ChatID); got != 5 {
		t.Fatalf("stats.lastSeq = %d, want 5", got)
	}
	if n, err := messages.CountDocuments(ctx, bson.M{"chatId": goneChat, "seq": bson.M{"$exists": true}}); err != nil || n != 0 {
		t.Fatalf("deleted chat's message numbered: %d, %v", n, err)
	}

	// A final run records the marker; after it, nothing is scanned or numbered.
	if _, err := BackfillMessageSeqs(ctx, true); err != nil {
		t.Fatalf("final run: %v", err)
	}
	if _, err := messages.InsertOne(ctx, bson.M{"_id": bson.NewObjectID(), "chatId": chat.ChatID}); err != nil {
		t.Fatalf("insert message: %v", err)
	}
	if n, err := BackfillMessageSeqs(ctx, false); err != nil || n != 0 {
		t.Fatalf("run after the marker numbered %d, %v; want 0", n, err)
	}
}

// #43 code review: the catch-up query must use chatId_1_seq_1. A partial index is
// used only when the query implies its filter, so a filter the query doesn't imply
// made every catch-up read and sort the whole chat.
func TestAfterSeqQueryUsesSeqIndex(t *testing.T) {
	ctx := useTestDB(t)
	if err := database.EnsureIndexes(ctx); err != nil {
		t.Fatalf("EnsureIndexes: %v", err)
	}
	chatID := bson.NewObjectID()
	if _, err := database.DB.Collection(string(constants.MessageColl)).InsertOne(ctx,
		bson.M{"_id": bson.NewObjectID(), "chatId": chatID, "seq": int64(1)}); err != nil {
		t.Fatalf("insert message: %v", err)
	}

	var plan bson.Raw
	err := database.DB.RunCommand(ctx, bson.D{
		{Key: "explain", Value: bson.D{
			{Key: "find", Value: string(constants.MessageColl)},
			{Key: "filter", Value: bson.M{"chatId": chatID, "seq": bson.M{"$gt": int64(0)}}},
			{Key: "sort", Value: bson.D{{Key: "seq", Value: 1}}},
		}},
		{Key: "verbosity", Value: "queryPlanner"},
	}).Decode(&plan)
	if err != nil {
		t.Fatalf("explain: %v", err)
	}
	winning := plan.Lookup("queryPlanner", "winningPlan").String()
	if !strings.Contains(winning, "chatId_1_seq_1") || strings.Contains(winning, "SORT") {
		t.Fatalf("catch-up plan doesn't read chatId_1_seq_1 in order: %s", winning)
	}
}
