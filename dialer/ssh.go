package dialer

import (
	"crypto/rsa"
	"fmt"
	"net"
	"net/url"

	"github.com/codingeasygo/bsck/dialer/syscall"
	"github.com/codingeasygo/util/xhash"
	"github.com/codingeasygo/util/xmap"
	"github.com/gliderlabs/ssh"

	gossh "golang.org/x/crypto/ssh"
)

type SshDialer struct {
	conf    xmap.M
	server  *ssh.Server
	accpter chan net.Conn
}

// NewSshDialer will return new SshDialer
func NewSshDialer() (dialer *SshDialer) {
	dialer = &SshDialer{
		conf:    xmap.M{},
		accpter: make(chan net.Conn, 8),
	}
	return
}

// Name will return dialer name
func (s *SshDialer) Name() string {
	return "ssh"
}

func (s *SshDialer) Network() string {
	return "ssh"
}

func (s *SshDialer) String() string {
	return "ssh"
}

func (s *SshDialer) Addr() net.Addr {
	return s
}

// Bootstrap the dialer
func (s *SshDialer) Bootstrap(options xmap.M) error {
	s.conf = options
	// signer
	cert, err := LoadX509KeyPair(s.conf.StrDef(".", "dir"), s.conf.StrDef("bsrouter.pem", "cert"), s.conf.StrDef("bsrouter.key", "key"))
	if err != nil {
		return err
	}
	privateKey := cert.PrivateKey.(*rsa.PrivateKey)
	signer, _ := gossh.NewSignerFromKey(privateKey)
	s.server = &ssh.Server{
		HostSigners: []ssh.Signer{signer},
		ServerConfigCallback: func(ctx ssh.Context) *gossh.ServerConfig {
			return &gossh.ServerConfig{
				NoClientAuth: len(s.conf.StrDef("", "auth")) < 1,
			}
		},
		PasswordHandler:   s.checkPassword,
		SubsystemHandlers: syscall.SubsystemSSH(),
		Handler:           syscall.HandlerSSH,
	}
	go s.server.Serve(s)
	return nil
}

func (s *SshDialer) checkPassword(ctx ssh.Context, password string) bool {
	auth := s.conf.StrDef("", "auth")
	return len(auth) < 1 || xhash.SHA1([]byte(password)) == auth
}

// Options is options getter
func (s *SshDialer) Options() xmap.M {
	return s.conf
}

// Matched will return whetheer uri is invalid
func (s *SshDialer) Matched(uri string) bool {
	target, err := url.Parse(uri)
	return err == nil && target.Scheme == "ssh" && target.Host == "server"
}

func (s *SshDialer) Accept() (conn net.Conn, err error) {
	conn = <-s.accpter
	if conn == nil {
		err = fmt.Errorf("closed")
	}
	return
}

// Dial one ssh connection.
func (s *SshDialer) Dial(channel Channel, sid uint16, uri string) (raw Conn, err error) {
	conn, raw := net.Pipe()
	s.accpter <- conn
	return
}

// Shutdown will shutdown dial
func (s *SshDialer) Shutdown() (err error) {
	s.accpter <- nil
	return
}

func (s *SshDialer) Close() (err error) {
	s.accpter <- nil
	return
}
