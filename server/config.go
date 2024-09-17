package main

type Config struct {
	LogLevel   string `env:"LOG_LEVEL" default:"INFO"`
	ListenAddr string `env:"LISTEN_ADDR" default:":220"`
	ServerKey  string `env:"SERVER_KEY"`
	ServerCert string `env:"SERVER_CERT"`
	RootCA     string `env:"ROOT_CA"`
}
