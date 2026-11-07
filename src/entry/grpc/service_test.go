package grpcserver

import (
	"context"
	"errors"
	"math"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"

	pb "qabz/api/pb/qabz"
	"qabz/core/billtype"
)

// fakeLister records the language of each call and returns fixed rows or an error.
type fakeLister struct {
	mu    sync.Mutex
	rows  []billtype.Service
	err   error
	langs []string
	hook  func(ctx context.Context) // runs inside List (for example to cancel ctx)
}

func (f *fakeLister) List(ctx context.Context, lang string) ([]billtype.Service, error) {
	f.mu.Lock()
	f.langs = append(f.langs, lang)
	hook := f.hook
	f.mu.Unlock()
	if hook != nil {
		hook(ctx)
	}
	return f.rows, f.err
}

var testRows = []billtype.Service{
	{Name: "Water", Type: "water", Tag: "new", IsActive: true, Inquiry: true, Icon: "w.svg"},
	{Name: "Gas", Type: "gas", Tag: "", IsActive: false, Inquiry: false, Icon: ""},
}

func TestGetMapsEveryField(t *testing.T) {
	svc := NewService(ServiceDeps{Services: &fakeLister{rows: testRows}, ServiceID: 105})
	res, err := svc.Get(context.Background(), &emptypb.Empty{})
	if !assert.NoError(t, err) || !assert.NotNil(t, res) || !assert.Len(t, res.GetServices(), 2) {
		return
	}
	assert.Equal(t, int32(105), res.GetServiceId())
	for i, want := range testRows {
		got := res.GetServices()[i]
		if !assert.NotNil(t, got) {
			continue
		}
		assert.Equal(t, want.Name, got.GetName())
		assert.Equal(t, want.Type, got.GetType())
		assert.Equal(t, want.Tag, got.GetTag())
		assert.Equal(t, want.IsActive, got.GetIsActive())
		assert.Equal(t, want.Inquiry, got.GetInquiry(), "inquiry is sent as stored (not forced to true like bill)")
		assert.Equal(t, want.Icon, got.GetIcon())
	}
}

func TestGetLanguage(t *testing.T) {
	cases := map[string]struct {
		ctx  context.Context
		want string
	}{
		"no metadata":     {context.Background(), billtype.DefaultLang},
		"no header":       {metadata.NewIncomingContext(context.Background(), metadata.Pairs("x", "y")), billtype.DefaultLang},
		"empty header":    {metadata.NewIncomingContext(context.Background(), metadata.Pairs(langHeader, "")), billtype.DefaultLang},
		"gateway header":  {metadata.NewIncomingContext(context.Background(), metadata.Pairs(langHeader, "en-US")), "en-US"},
		"first value":     {metadata.NewIncomingContext(context.Background(), metadata.Pairs(langHeader, "en-US", langHeader, "fa-IR")), "en-US"},
		"raw, not parsed": {metadata.NewIncomingContext(context.Background(), metadata.Pairs(langHeader, "en-US,fa;q=0.5")), "en-US,fa;q=0.5"},
		"plain key only":  {metadata.NewIncomingContext(context.Background(), metadata.Pairs("accept-language", "en-US")), billtype.DefaultLang},
	}
	for name, tc := range cases {
		f := &fakeLister{}
		_, err := NewService(ServiceDeps{Services: f}).Get(tc.ctx, &emptypb.Empty{})
		assert.NoError(t, err, name)
		assert.Equal(t, []string{tc.want}, f.langs, name)
	}
}

func TestGetEmptyList(t *testing.T) {
	for _, rows := range [][]billtype.Service{nil, {}} {
		res, err := NewService(ServiceDeps{Services: &fakeLister{rows: rows}, ServiceID: 7}).Get(context.Background(), nil)
		if assert.NoError(t, err) && assert.NotNil(t, res) {
			assert.Empty(t, res.GetServices())
			assert.Equal(t, int32(7), res.GetServiceId())
		}
	}
}

// errLog records the error lines.
type errLog struct {
	mu    sync.Mutex
	lines []string
	kvs   [][]any
}

func (l *errLog) Error(msg string, kv ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.lines = append(l.lines, msg)
	l.kvs = append(l.kvs, kv)
}

func TestGetErrorIsInternal(t *testing.T) {
	cause := errors.New(`relation "services" does not exist`)
	log := &errLog{}
	svc := NewService(ServiceDeps{Services: &fakeLister{err: cause}, Logger: log})
	res, err := svc.Get(context.Background(), &emptypb.Empty{})
	assert.Nil(t, res)
	if assert.Error(t, err) {
		assert.Equal(t, codes.Internal, status.Code(err))
		assert.NotContains(t, err.Error(), "services", "the cause is not sent to the client")
	}
	if assert.Len(t, log.lines, 1, "one error line for one failed read") && assert.Len(t, log.kvs, 1) {
		assert.Equal(t, "services: list failed", log.lines[0])
		assert.Equal(t, []any{"error", cause}, log.kvs[0], "the log has the cause")
	}

	// Without a logger the answer is the same.
	_, err = NewService(ServiceDeps{Services: &fakeLister{err: cause}}).Get(context.Background(), &emptypb.Empty{})
	assert.Equal(t, codes.Internal, status.Code(err))
}

func TestGetContext(t *testing.T) {
	f := &fakeLister{rows: testRows}
	svc := NewService(ServiceDeps{Services: f})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	res, err := svc.Get(ctx, &emptypb.Empty{})
	assert.Nil(t, res)
	if assert.Error(t, err) {
		assert.Equal(t, codes.Canceled, status.Code(err))
	}
	assert.Empty(t, f.langs, "a canceled call does not read the list")

	res, err = svc.Get(nil, &emptypb.Empty{}) //nolint:staticcheck // a nil context is the case under test
	if assert.NoError(t, err) && assert.NotNil(t, res) {
		assert.Len(t, res.GetServices(), 2)
	}

	// The list fails because the call was canceled meanwhile: the answer is Canceled, not Internal.
	ctx, cancel = context.WithCancel(context.Background())
	f2 := &fakeLister{err: context.Canceled, hook: func(context.Context) { cancel() }}
	log := &errLog{}
	_, err = NewService(ServiceDeps{Services: f2, Logger: log}).Get(ctx, &emptypb.Empty{})
	if assert.Error(t, err) {
		assert.Equal(t, codes.Canceled, status.Code(err))
	}
	assert.Empty(t, log.lines, "a call the client canceled is not logged")
}

func TestGetWithoutListerIsUnimplemented(t *testing.T) {
	res, err := NewService(ServiceDeps{}).Get(context.Background(), &emptypb.Empty{})
	assert.Nil(t, res)
	if assert.Error(t, err) {
		assert.Equal(t, codes.Unimplemented, status.Code(err))
	}
}

func TestGetOverGRPC(t *testing.T) {
	f := &fakeLister{rows: testRows}
	s, err := New("127.0.0.1:0", RegisterService(NewService(ServiceDeps{Services: f, ServiceID: 105})))
	if !assert.NoError(t, err) || !assert.NotNil(t, s) || !assert.NoError(t, s.Start(context.Background())) {
		t.FailNow()
	}
	t.Cleanup(func() { _ = s.Stop(context.Background()) })
	ctx := metadata.AppendToOutgoingContext(context.Background(), langHeader, "en-US")
	res, err := client(t, s).Get(ctx, &emptypb.Empty{})
	if assert.NoError(t, err) && assert.NotNil(t, res) {
		assert.Equal(t, int32(105), res.GetServiceId())
		assert.Len(t, res.GetServices(), 2)
	}
	f.mu.Lock()
	assert.Equal(t, []string{"en-US"}, f.langs)
	f.mu.Unlock()
}

func TestServiceIDFromConfig(t *testing.T) {
	for _, id := range []int{0, 105, math.MaxInt32} {
		got, err := ServiceIDFromConfig(id)
		assert.NoError(t, err)
		assert.Equal(t, int32(id), got)
	}
	for _, id := range []int{-1, math.MaxInt32 + 1} {
		_, err := ServiceIDFromConfig(id)
		assert.Error(t, err, id)
	}
}

var _ pb.ServiceServer = (*Service)(nil)
