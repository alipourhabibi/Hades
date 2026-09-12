package notification

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	identityv1 "github.com/alipourhabibi/Hades/api/gen/api/identity/v1"
	"github.com/alipourhabibi/Hades/config"
	"github.com/alipourhabibi/Hades/internal/hades/constants"
	notificationdb "github.com/alipourhabibi/Hades/internal/hades/storage/db/notification"
	"github.com/alipourhabibi/Hades/internal/hades/testsupport"
	"github.com/alipourhabibi/Hades/utils/log"
)

type fixture struct {
	env   *testsupport.Env
	h     *Handler
	alice *identityv1.User
	bob   *identityv1.User
}

func newFixture(t *testing.T) *fixture {
	t.Helper()

	env := testsupport.NewEnv(t)
	logger, err := log.NewWithConfig(config.Logger{Level: "error", Output: "stdout"})
	require.NoError(t, err)

	return &fixture{
		env:   env,
		alice: env.CreateUser(t, "alice", "alice@example.com"),
		bob:   env.CreateUser(t, "bob", "bob@example.com"),
		h: &Handler{
			logger:              logger,
			notificationStorage: env.DB.Notification(),
		},
	}
}

func ctxFor(user *identityv1.User) context.Context {
	return context.WithValue(context.Background(), constants.ContextKeyUser, user)
}

func TestListNotifications_ReturnsOnlyTheCallersOwn(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	require.NoError(t, f.env.DB.Notification().Create(ctx, f.alice.Id, notificationdb.TypeCommitPushed, "for alice", "", "c-1"))
	require.NoError(t, f.env.DB.Notification().Create(ctx, f.bob.Id, notificationdb.TypeSDKFailed, "for bob", "", "job-1"))

	resp, err := f.h.ListNotifications(ctxFor(f.alice), connect.NewRequest(&identityv1.ListNotificationsRequest{}))

	require.NoError(t, err)
	require.Len(t, resp.Msg.Notifications, 1)
	assert.Equal(t, "for alice", resp.Msg.Notifications[0].Title)
	assert.Equal(t, notificationdb.TypeCommitPushed, resp.Msg.Notifications[0].Type)
	assert.False(t, resp.Msg.Notifications[0].Read)
}

func TestListNotifications_AnonymousIsUnauthenticated(t *testing.T) {
	f := newFixture(t)

	_, err := f.h.ListNotifications(context.Background(), connect.NewRequest(&identityv1.ListNotificationsRequest{}))

	assert.Equal(t, connect.CodeUnauthenticated, connect.CodeOf(err))
}

func TestListNotifications_EmptyForAUserWithNone(t *testing.T) {
	f := newFixture(t)

	resp, err := f.h.ListNotifications(ctxFor(f.alice), connect.NewRequest(&identityv1.ListNotificationsRequest{}))

	require.NoError(t, err)
	assert.Empty(t, resp.Msg.Notifications)
}

func TestMarkNotificationRead_MarksTheCallersOwn(t *testing.T) {
	f := newFixture(t)
	require.NoError(t, f.env.DB.Notification().Create(context.Background(), f.alice.Id,
		notificationdb.TypeCommitPushed, "for alice", "", "c-1"))

	listed, err := f.h.ListNotifications(ctxFor(f.alice), connect.NewRequest(&identityv1.ListNotificationsRequest{}))
	require.NoError(t, err)
	id := listed.Msg.Notifications[0].Id

	_, err = f.h.MarkNotificationRead(ctxFor(f.alice), connect.NewRequest(&identityv1.MarkNotificationReadRequest{Id: id}))
	require.NoError(t, err)

	listed, err = f.h.ListNotifications(ctxFor(f.alice), connect.NewRequest(&identityv1.ListNotificationsRequest{}))
	require.NoError(t, err)
	assert.True(t, listed.Msg.Notifications[0].Read)
}

// TestMarkNotificationRead_AnotherUsersIdChangesNothing pins the shape of the
// ownership check: the UPDATE is scoped by user id, so a foreign id matches no
// row. Succeeding without an error is the intended answer, because reporting
// NotFound would confirm which ids exist.
func TestMarkNotificationRead_AnotherUsersIdChangesNothing(t *testing.T) {
	f := newFixture(t)
	require.NoError(t, f.env.DB.Notification().Create(context.Background(), f.alice.Id,
		notificationdb.TypeCommitPushed, "for alice", "", "c-1"))

	listed, err := f.h.ListNotifications(ctxFor(f.alice), connect.NewRequest(&identityv1.ListNotificationsRequest{}))
	require.NoError(t, err)
	aliceID := listed.Msg.Notifications[0].Id

	_, err = f.h.MarkNotificationRead(ctxFor(f.bob), connect.NewRequest(&identityv1.MarkNotificationReadRequest{Id: aliceID}))
	require.NoError(t, err)

	listed, err = f.h.ListNotifications(ctxFor(f.alice), connect.NewRequest(&identityv1.ListNotificationsRequest{}))
	require.NoError(t, err)
	assert.False(t, listed.Msg.Notifications[0].Read, "bob must not be able to mark alice's notification read")
}

func TestMarkNotificationRead_AnonymousIsUnauthenticated(t *testing.T) {
	f := newFixture(t)

	_, err := f.h.MarkNotificationRead(context.Background(), connect.NewRequest(&identityv1.MarkNotificationReadRequest{Id: "x"}))

	assert.Equal(t, connect.CodeUnauthenticated, connect.CodeOf(err))
}
