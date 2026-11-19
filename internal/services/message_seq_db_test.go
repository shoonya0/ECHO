package services

import (
	"context"
	"errors"
	"slices"
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

// #43: messages saved without a seq (before #43, or by an older server copy during
// a deploy) are numbered oldest first after the chat's newest numbered message.
// A second run changes nothing, and messages of a deleted chat are left alone.
func TestBackfillMessageSeqs(t *testing.T) {
	ctx := useTestDB(t)
	if err := database.EnsureIndexes(ctx); err != nil {
		t.Fatalf("EnsureIndexes: %v", err)
	}
	chats := database.DB.Collection(string(constants.ChatColl))
	messages := database.DB.Collection(string(constants.MessageColl))

	chatID, goneChat := bson.NewObjectID(), bson.NewObjectID()
	if _, err := chats.InsertOne(ctx, bson.M{"_id": chatID, "stats": bson.M{"lastSeq": int64(2)}}); err != nil {
		t.Fatalf("insert chat: %v", err)
	}
	// IDs in creation order: the numbered ones, then three without a seq.
	ids := make([]bson.ObjectID, 5)
	for i := range ids {
		ids[i] = bson.NewObjectID()
	}
	docs := []any{
		bson.M{"_id": ids[0], "chatId": chatID, "seq": int64(1)},
		bson.M{"_id": ids[1], "chatId": chatID, "seq": int64(2)},
		bson.M{"_id": ids[4], "chatId": chatID}, // inserted out of order on purpose
		bson.M{"_id": ids[2], "chatId": chatID},
		bson.M{"_id": ids[3], "chatId": chatID},
		bson.M{"_id": bson.NewObjectID(), "chatId": goneChat},
	}
	if _, err := messages.InsertMany(ctx, docs); err != nil {
		t.Fatalf("insert messages: %v", err)
	}

	for run, want := range []int64{3, 0} {
		n, err := BackfillMessageSeqs(ctx)
		if err != nil {
			t.Fatalf("run %d: BackfillMessageSeqs: %v", run+1, err)
		}
		if n != want {
			t.Fatalf("run %d: numbered %d, want %d", run+1, n, want)
		}
	}

	for i, id := range ids {
		var m models.Message
		if err := messages.FindOne(ctx, bson.M{"_id": id}).Decode(&m); err != nil {
			t.Fatalf("read message %d: %v", i, err)
		}
		if m.Seq != int64(i+1) {
			t.Fatalf("message %d has seq %d, want %d", i, m.Seq, i+1)
		}
	}
	if got := chatLastSeq(t, ctx, chatID); got != 5 {
		t.Fatalf("stats.lastSeq = %d, want 5", got)
	}
	if n, err := messages.CountDocuments(ctx, bson.M{"chatId": goneChat, "seq": bson.M{"$exists": true}}); err != nil || n != 0 {
		t.Fatalf("deleted chat's message numbered: %d, %v", n, err)
	}
}
