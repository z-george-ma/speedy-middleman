package main

type Config struct {
	LogLevel   string `env:"LOG_LEVEL" default:"INFO"`
	ListenAddr string `env:"LISTEN_ADDR" default:"localhost:8080"`
	RootCA     string `env:"ROOT_CA"`
	ClientCert string `env:"CLIENT_CERT"`
	ClientKey  string `env:"CLIENT_KEY"`
	RemoteUrl  string `env:"REMOTE_URL"`
}
