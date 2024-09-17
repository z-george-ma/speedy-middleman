package lib

import (
	"context"
	"net"
	"time"

	"github.com/goccy/go-json"
	fiber "github.com/gofiber/fiber/v2"
)

type Fiber struct {
	*fiber.App
	closed chan error
	stop   func() bool
}

type FiberConfig = fiber.Config

func NewFiber(
	defineRoute func(*fiber.App) error,
	config ...fiber.Config,
) (ret *Fiber, err error) {
	newConfig := fiber.Config{
		DisableStartupMessage: true,
		JSONEncoder:           json.Marshal,
		JSONDecoder:           json.Unmarshal,
	}

	if config != nil {
		newConfig = config[0]

		if newConfig.JSONEncoder == nil {
			newConfig.JSONEncoder = json.Marshal
		}

		if newConfig.JSONDecoder == nil {
			newConfig.JSONDecoder = json.Unmarshal
		}
	}

	ret = &Fiber{
		App:    fiber.New(newConfig),
		closed: make(chan error, 1),
	}

	err = defineRoute(ret.App)
	return
}

func (f *Fiber) Start(ctx context.Context, addr string, shutdownTimeout time.Duration) error {
	f.stop = context.AfterFunc(ctx, func() {
		select {
		case f.closed <- f.App.ShutdownWithTimeout(shutdownTimeout):
		default:
		}
	})

	err := f.App.Listen(addr)

	if err != nil {
		return err
	}
	return <-f.closed
}

func (f *Fiber) StartListener(ctx context.Context, listener net.Listener, shutdownTimeout time.Duration) error {
	f.stop = context.AfterFunc(ctx, func() {
		select {
		case f.closed <- f.App.ShutdownWithTimeout(shutdownTimeout):
		default:
		}
	})

	err := f.App.Listener(listener)

	if err != nil {
		return err
	}
	return <-f.closed
}

func (f *Fiber) Close(shutdownTimeout time.Duration) error {
	if f.stop != nil {
		f.stop()
	}
	err := f.App.ShutdownWithTimeout(shutdownTimeout)

	select {
	case f.closed <- err:
	default:
	}
	close(f.closed)
	return err
}
