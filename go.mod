module github.com/codingeasygo/bsck

go 1.21.1

toolchain go1.21.4

require (
	github.com/codingeasygo/tun2conn v0.0.0-20231213091254-2140cd9f2c51
	github.com/codingeasygo/util v0.0.0-20231206062002-1ce2f004b7d9
	github.com/codingeasygo/web v0.0.0-20230907002627-38429b961da0
	github.com/creack/pty v1.1.20
	github.com/gliderlabs/ssh v0.3.5
	github.com/pkg/sftp v1.13.6
	github.com/quic-go/quic-go v0.40.0
	github.com/songgao/water v0.0.0-20200317203138-2b4b6d7c09d8
	golang.org/x/crypto v0.16.0
	golang.org/x/mobile v0.0.0-20231127183840-76ac6878050a
	golang.org/x/net v0.19.0
	gvisor.dev/gvisor v0.0.0-20231202080848-1f7806d17489
)

require (
	github.com/anmitsu/go-shlex v0.0.0-20200514113438-38f4b401e2be // indirect
	github.com/go-task/slim-sprig v0.0.0-20230315185526-52ccab3ef572 // indirect
	github.com/google/btree v1.1.2 // indirect
	github.com/google/pprof v0.0.0-20210407192527-94a9f03dee38 // indirect
	github.com/kr/fs v0.1.0 // indirect
	github.com/onsi/ginkgo/v2 v2.9.5 // indirect
	github.com/quic-go/qtls-go1-20 v0.4.1 // indirect
	go.uber.org/mock v0.3.0 // indirect
	golang.org/x/exp v0.0.0-20230725093048-515e97ebf090 // indirect
	golang.org/x/mod v0.14.0 // indirect
	golang.org/x/sys v0.15.0 // indirect
	golang.org/x/time v0.3.0 // indirect
	golang.org/x/tools v0.16.0 // indirect
)

replace gvisor.dev/gvisor v0.0.0-20231202080848-1f7806d17489 => github.com/codingeasygo/gvisor v0.0.0-20231203111534-4a1d1d9214fa

// replace github.com/codingeasygo/tun2conn => ../tun2conn

// replace github.com/codingeasygo/util => ../util
