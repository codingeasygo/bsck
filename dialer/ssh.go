package dialer

import (
	"crypto/rsa"
	"fmt"
	"io"
	"net"
	"net/url"

	"github.com/codingeasygo/util/xhash"
	"github.com/codingeasygo/util/xio"
	"github.com/codingeasygo/util/xmap"
	"github.com/gliderlabs/ssh"
	"github.com/pkg/sftp"

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
	return "echo"
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
	cert, err := LoadX509KeyPair(s.conf.StrDef(".", "dir"), s.conf.StrDef("bsrouter.pem"), s.conf.StrDef("bsrouter.key"))
	if err != nil {
		return err
	}
	privateKey, ok := cert.PrivateKey.(*rsa.PrivateKey)
	if !ok {
		err = fmt.Errorf("not rsa private key")
		return err
	}
	signer, err := gossh.NewSignerFromKey(privateKey)
	if err != nil {
		return err
	}
	s.server = &ssh.Server{
		HostSigners: []ssh.Signer{signer},
		ServerConfigCallback: func(ctx ssh.Context) *gossh.ServerConfig {
			return &gossh.ServerConfig{
				NoClientAuth: true,
			}
		},
		SubsystemHandlers: map[string]ssh.SubsystemHandler{
			"sftp": func(s ssh.Session) {
				server, err := sftp.NewServer(s)
				if err != nil {
					return
				}
				if err := server.Serve(); err == io.EOF {
					server.Close()
				}
			},
		},
		Handler: sshHandler,
	}
	if len(s.conf.StrDef("", "auth")) > 0 {
		s.server.PasswordHandler = s.checkPassword
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
	conn, raw, err := xio.CreatePipedConn()
	if err == nil {
		s.accpter <- conn
	}
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
