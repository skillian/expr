"".(*S).initAorB STEXT size=147 args=0x8 locals=0x8 funcid=0x0 align=0x0
	0x0000 00000 (./main.go:16)	TEXT	"".(*S).initAorB(SB), ABIInternal, $8-8
	0x0000 00000 (./main.go:16)	CMPQ	SP, 16(R14)
	0x0004 00004 (./main.go:16)	PCDATA	$0, $-2
	0x0004 00004 (./main.go:16)	JLS	127
	0x0006 00006 (./main.go:16)	PCDATA	$0, $-1
	0x0006 00006 (./main.go:16)	SUBQ	$8, SP
	0x000a 00010 (./main.go:16)	MOVQ	BP, (SP)
	0x000e 00014 (./main.go:16)	LEAQ	(SP), BP
	0x0012 00018 (./main.go:16)	FUNCDATA	$0, gclocals┬╖1a65e721a2ccc325b382662e7ffee780(SB)
	0x0012 00018 (./main.go:16)	FUNCDATA	$1, gclocals┬╖69c1753bd5f81501d95132d08af04464(SB)
	0x0012 00018 (./main.go:16)	FUNCDATA	$5, "".(*S).initAorB.arginfo1(SB)
	0x0012 00018 (./main.go:16)	FUNCDATA	$6, "".(*S).initAorB.argliveinfo(SB)
	0x0012 00018 (./main.go:16)	PCDATA	$3, $1
	0x0012 00018 (./main.go:17)	MOVQ	AX, CX
	0x0015 00021 (./main.go:17)	BTL	$5, AX
	0x0019 00025 (./main.go:17)	JCC	78
	0x001b 00027 (./main.go:18)	TESTB	AL, (CX)
	0x001d 00029 (./main.go:18)	PCDATA	$0, $-2
	0x001d 00029 (./main.go:18)	CMPL	runtime.writeBarrier(SB), $0
	0x0024 00036 (./main.go:18)	JNE	51
	0x0026 00038 (./main.go:18)	LEAQ	"".a┬╖f(SB), AX
	0x002d 00045 (./main.go:18)	MOVQ	AX, 8(CX)
	0x0031 00049 (./main.go:18)	JMP	69
	0x0033 00051 (./main.go:18)	LEAQ	8(CX), DI
	0x0037 00055 (./main.go:18)	LEAQ	"".a┬╖f(SB), AX
	0x003e 00062 (./main.go:18)	NOP
	0x0040 00064 (./main.go:18)	CALL	runtime.gcWriteBarrier(SB)
	0x0045 00069 (./main.go:19)	PCDATA	$0, $-1
	0x0045 00069 (./main.go:19)	MOVQ	(SP), BP
	0x0049 00073 (./main.go:19)	ADDQ	$8, SP
	0x004d 00077 (./main.go:19)	RET
	0x004e 00078 (./main.go:21)	TESTB	AL, (CX)
	0x0050 00080 (./main.go:21)	PCDATA	$0, $-2
	0x0050 00080 (./main.go:21)	CMPL	runtime.writeBarrier(SB), $0
	0x0057 00087 (./main.go:21)	JNE	102
	0x0059 00089 (./main.go:21)	LEAQ	"".b┬╖f(SB), AX
	0x0060 00096 (./main.go:21)	MOVQ	AX, 8(CX)
	0x0064 00100 (./main.go:21)	JMP	118
	0x0066 00102 (./main.go:21)	LEAQ	8(CX), DI
	0x006a 00106 (./main.go:21)	LEAQ	"".b┬╖f(SB), AX
	0x0071 00113 (./main.go:21)	CALL	runtime.gcWriteBarrier(SB)
	0x0076 00118 (./main.go:22)	PCDATA	$0, $-1
	0x0076 00118 (./main.go:22)	MOVQ	(SP), BP
	0x007a 00122 (./main.go:22)	ADDQ	$8, SP
	0x007e 00126 (./main.go:22)	RET
	0x007f 00127 (./main.go:22)	NOP
	0x007f 00127 (./main.go:16)	PCDATA	$1, $-1
	0x007f 00127 (./main.go:16)	PCDATA	$0, $-2
	0x007f 00127 (./main.go:16)	MOVQ	AX, 8(SP)
	0x0084 00132 (./main.go:16)	CALL	runtime.morestack_noctxt(SB)
	0x0089 00137 (./main.go:16)	MOVQ	8(SP), AX
	0x008e 00142 (./main.go:16)	PCDATA	$0, $-1
	0x008e 00142 (./main.go:16)	JMP	0
	0x0000 49 3b 66 10 76 79 48 83 ec 08 48 89 2c 24 48 8d  I;f.vyH...H.,$H.
	0x0010 2c 24 48 89 c1 0f ba e0 05 73 33 84 01 83 3d 00  ,$H......s3...=.
	0x0020 00 00 00 00 75 0d 48 8d 05 00 00 00 00 48 89 41  ....u.H......H.A
	0x0030 08 eb 12 48 8d 79 08 48 8d 05 00 00 00 00 66 90  ...H.y.H......f.
	0x0040 e8 00 00 00 00 48 8b 2c 24 48 83 c4 08 c3 84 01  .....H.,$H......
	0x0050 83 3d 00 00 00 00 00 75 0d 48 8d 05 00 00 00 00  .=.....u.H......
	0x0060 48 89 41 08 eb 10 48 8d 79 08 48 8d 05 00 00 00  H.A...H.y.H.....
	0x0070 00 e8 00 00 00 00 48 8b 2c 24 48 83 c4 08 c3 48  ......H.,$H....H
	0x0080 89 44 24 08 e8 00 00 00 00 48 8b 44 24 08 e9 6d  .D$......H.D$..m
	0x0090 ff ff ff                                         ...
	rel 31+4 t=14 runtime.writeBarrier+-1
	rel 41+4 t=14 "".a┬╖f+0
	rel 58+4 t=14 "".a┬╖f+0
	rel 65+4 t=7 runtime.gcWriteBarrier+0
	rel 82+4 t=14 runtime.writeBarrier+-1
	rel 92+4 t=14 "".b┬╖f+0
	rel 109+4 t=14 "".b┬╖f+0
	rel 114+4 t=7 runtime.gcWriteBarrier+0
	rel 133+4 t=7 runtime.morestack_noctxt+0
"".main STEXT size=165 args=0x0 locals=0x50 funcid=0x0 align=0x0
	0x0000 00000 (./main.go:24)	TEXT	"".main(SB), ABIInternal, $80-0
	0x0000 00000 (./main.go:24)	CMPQ	SP, 16(R14)
	0x0004 00004 (./main.go:24)	PCDATA	$0, $-2
	0x0004 00004 (./main.go:24)	JLS	150
	0x000a 00010 (./main.go:24)	PCDATA	$0, $-1
	0x000a 00010 (./main.go:24)	SUBQ	$80, SP
	0x000e 00014 (./main.go:24)	MOVQ	BP, 72(SP)
	0x0013 00019 (./main.go:24)	LEAQ	72(SP), BP
	0x0018 00024 (./main.go:24)	FUNCDATA	$0, gclocals┬╖7d2d5fca80364273fb07d5820a76fef4(SB)
	0x0018 00024 (./main.go:24)	FUNCDATA	$1, gclocals┬╖ef22736e31ca7e9ff10587cea75d17ff(SB)
	0x0018 00024 (./main.go:24)	FUNCDATA	$2, "".main.stkobj(SB)
	0x0018 00024 (./main.go:25)	MOVUPS	X15, ""..autotmp_10+56(SP)
	0x001e 00030 (./main.go:25)	MOVUPS	X15, ""..autotmp_10+56(SP)
	0x0024 00036 (./main.go:26)	LEAQ	""..autotmp_10+56(SP), AX
	0x0029 00041 (./main.go:26)	PCDATA	$1, $1
	0x0029 00041 (./main.go:26)	CALL	"".(*S).initAorB(SB)
	0x002e 00046 (./main.go:27)	MOVQ	""..autotmp_10+64(SP), DX
	0x0033 00051 (./main.go:27)	MOVQ	(DX), CX
	0x0036 00054 (./main.go:27)	LEAQ	go.string."test"(SB), AX
	0x003d 00061 (./main.go:27)	MOVL	$4, BX
	0x0042 00066 (./main.go:27)	MOVQ	CX, SI
	0x0045 00069 (./main.go:27)	MOVL	$65261, CX
	0x004a 00074 (./main.go:27)	PCDATA	$1, $0
	0x004a 00074 (./main.go:27)	CALL	SI
	0x004c 00076 (./main.go:27)	MOVUPS	X15, ""..autotmp_12+40(SP)
	0x0052 00082 (./main.go:27)	MOVL	X0, AX
	0x0056 00086 (./main.go:27)	PCDATA	$1, $2
	0x0056 00086 (./main.go:27)	CALL	runtime.convT32(SB)
	0x005b 00091 (./main.go:27)	LEAQ	type.float32(SB), CX
	0x0062 00098 (./main.go:27)	MOVQ	CX, ""..autotmp_12+40(SP)
	0x0067 00103 (./main.go:27)	MOVQ	AX, ""..autotmp_12+48(SP)
	0x006c 00108 (<unknown line number>)	NOP
	0x006c 00108 ($GOROOT\src\fmt\print.go:274)	MOVQ	os.Stdout(SB), BX
	0x0073 00115 ($GOROOT\src\fmt\print.go:274)	LEAQ	go.itab.*os.File,io.Writer(SB), AX
	0x007a 00122 ($GOROOT\src\fmt\print.go:274)	LEAQ	""..autotmp_12+40(SP), CX
	0x007f 00127 ($GOROOT\src\fmt\print.go:274)	MOVL	$1, DI
	0x0084 00132 ($GOROOT\src\fmt\print.go:274)	MOVQ	DI, SI
	0x0087 00135 ($GOROOT\src\fmt\print.go:274)	PCDATA	$1, $0
	0x0087 00135 ($GOROOT\src\fmt\print.go:274)	CALL	fmt.Fprintln(SB)
	0x008c 00140 (./main.go:28)	MOVQ	72(SP), BP
	0x0091 00145 (./main.go:28)	ADDQ	$80, SP
	0x0095 00149 (./main.go:28)	RET
	0x0096 00150 (./main.go:28)	NOP
	0x0096 00150 (./main.go:24)	PCDATA	$1, $-1
	0x0096 00150 (./main.go:24)	PCDATA	$0, $-2
	0x0096 00150 (./main.go:24)	CALL	runtime.morestack_noctxt(SB)
	0x009b 00155 (./main.go:24)	PCDATA	$0, $-1
	0x009b 00155 (./main.go:24)	NOP
	0x00a0 00160 (./main.go:24)	JMP	0
	0x0000 49 3b 66 10 0f 86 8c 00 00 00 48 83 ec 50 48 89  I;f.......H..PH.
	0x0010 6c 24 48 48 8d 6c 24 48 44 0f 11 7c 24 38 44 0f  l$HH.l$HD..|$8D.
	0x0020 11 7c 24 38 48 8d 44 24 38 e8 00 00 00 00 48 8b  .|$8H.D$8.....H.
	0x0030 54 24 40 48 8b 0a 48 8d 05 00 00 00 00 bb 04 00  T$@H..H.........
	0x0040 00 00 48 89 ce b9 ed fe 00 00 ff d6 44 0f 11 7c  ..H.........D..|
	0x0050 24 28 66 0f 7e c0 e8 00 00 00 00 48 8d 0d 00 00  $(f.~......H....
	0x0060 00 00 48 89 4c 24 28 48 89 44 24 30 48 8b 1d 00  ..H.L$(H.D$0H...
	0x0070 00 00 00 48 8d 05 00 00 00 00 48 8d 4c 24 28 bf  ...H......H.L$(.
	0x0080 01 00 00 00 48 89 fe e8 00 00 00 00 48 8b 6c 24  ....H.......H.l$
	0x0090 48 48 83 c4 50 c3 e8 00 00 00 00 0f 1f 44 00 00  HH..P........D..
	0x00a0 e9 5b ff ff ff                                   .[...
	rel 3+0 t=23 type.float32+0
	rel 3+0 t=23 type.*os.File+0
	rel 42+4 t=7 "".(*S).initAorB+0
	rel 57+4 t=14 go.string."test"+0
	rel 74+0 t=10 +0
	rel 87+4 t=7 runtime.convT32+0
	rel 94+4 t=14 type.float32+0
	rel 111+4 t=14 os.Stdout+0
	rel 118+4 t=14 go.itab.*os.File,io.Writer+0
	rel 136+4 t=7 fmt.Fprintln+0
	rel 151+4 t=7 runtime.morestack_noctxt+0
"".a STEXT nosplit size=9 args=0x18 locals=0x0 funcid=0x0 align=0x0
	0x0000 00000 (./main.go:30)	TEXT	"".a(SB), NOSPLIT|ABIInternal, $0-24
	0x0000 00000 (./main.go:30)	FUNCDATA	$0, gclocals┬╖2a5305abe05176240e61b8620e19a815(SB)
	0x0000 00000 (./main.go:30)	FUNCDATA	$1, gclocals┬╖33cdeccccebe80329f1fdbee7f5874cb(SB)
	0x0000 00000 (./main.go:30)	FUNCDATA	$5, "".a.arginfo1(SB)
	0x0000 00000 (./main.go:30)	FUNCDATA	$6, "".a.argliveinfo(SB)
	0x0000 00000 (./main.go:30)	PCDATA	$3, $1
	0x0000 00000 (./main.go:31)	MOVSS	$f32.3fa00000(SB), X0
	0x0008 00008 (./main.go:31)	RET
	0x0000 f3 0f 10 05 00 00 00 00 c3                       .........
	rel 4+4 t=14 $f32.3fa00000+0
"".b STEXT nosplit size=9 args=0x18 locals=0x0 funcid=0x0 align=0x0
	0x0000 00000 (./main.go:34)	TEXT	"".b(SB), NOSPLIT|ABIInternal, $0-24
	0x0000 00000 (./main.go:34)	FUNCDATA	$0, gclocals┬╖2a5305abe05176240e61b8620e19a815(SB)
	0x0000 00000 (./main.go:34)	FUNCDATA	$1, gclocals┬╖33cdeccccebe80329f1fdbee7f5874cb(SB)
	0x0000 00000 (./main.go:34)	FUNCDATA	$5, "".b.arginfo1(SB)
	0x0000 00000 (./main.go:34)	FUNCDATA	$6, "".b.argliveinfo(SB)
	0x0000 00000 (./main.go:34)	PCDATA	$3, $1
	0x0000 00000 (./main.go:35)	MOVSS	$f32.40080000(SB), X0
	0x0008 00008 (./main.go:35)	RET
	0x0000 f3 0f 10 05 00 00 00 00 c3                       .........
	rel 4+4 t=14 $f32.40080000+0
go.cuinfo.packagename. SDWARFCUINFO dupok size=0
	0x0000 6d 61 69 6e                                      main
go.info.fmt.Println$abstract SDWARFABSFCN dupok size=42
	0x0000 05 66 6d 74 2e 50 72 69 6e 74 6c 6e 00 01 01 13  .fmt.Println....
	0x0010 61 00 00 00 00 00 00 13 6e 00 01 00 00 00 00 13  a.......n.......
	0x0020 65 72 72 00 01 00 00 00 00 00                    err.......
	rel 0+0 t=22 type.[]interface {}+0
	rel 0+0 t=22 type.error+0
	rel 0+0 t=22 type.int+0
	rel 19+4 t=31 go.info.[]interface {}+0
	rel 27+4 t=31 go.info.int+0
	rel 37+4 t=31 go.info.error+0
""..inittask SNOPTRDATA size=32
	0x0000 00 00 00 00 00 00 00 00 01 00 00 00 00 00 00 00  ................
	0x0010 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00  ................
	rel 24+8 t=1 fmt..inittask+0
go.string."test" SRODATA dupok size=4
	0x0000 74 65 73 74                                      test
go.itab.*os.File,io.Writer SRODATA dupok size=32
	0x0000 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00  ................
	0x0010 44 b5 f3 33 00 00 00 00 00 00 00 00 00 00 00 00  D..3............
	rel 0+8 t=1 type.io.Writer+0
	rel 8+8 t=1 type.*os.File+0
	rel 24+8 t=-32767 os.(*File).Write+0
runtime.memequal64┬╖f SRODATA dupok size=8
	0x0000 00 00 00 00 00 00 00 00                          ........
	rel 0+8 t=1 runtime.memequal64+0
runtime.gcbits.01 SRODATA dupok size=1
	0x0000 01                                               .
type..namedata.*func(string, int) float32- SRODATA dupok size=28
	0x0000 00 1a 2a 66 75 6e 63 28 73 74 72 69 6e 67 2c 20  ..*func(string, 
	0x0010 69 6e 74 29 20 66 6c 6f 61 74 33 32              int) float32
type.*func(string, int) float32 SRODATA dupok size=56
	0x0000 08 00 00 00 00 00 00 00 08 00 00 00 00 00 00 00  ................
	0x0010 b2 f6 b3 32 08 08 08 36 00 00 00 00 00 00 00 00  ...2...6........
	0x0020 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00  ................
	0x0030 00 00 00 00 00 00 00 00                          ........
	rel 24+8 t=1 runtime.memequal64┬╖f+0
	rel 32+8 t=1 runtime.gcbits.01+0
	rel 40+4 t=5 type..namedata.*func(string, int) float32-+0
	rel 48+8 t=1 type.func(string, int) float32+0
type.func(string, int) float32 SRODATA dupok size=80
	0x0000 08 00 00 00 00 00 00 00 08 00 00 00 00 00 00 00  ................
	0x0010 64 db d7 c4 02 08 08 33 00 00 00 00 00 00 00 00  d......3........
	0x0020 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00  ................
	0x0030 02 00 01 00 00 00 00 00 00 00 00 00 00 00 00 00  ................
	0x0040 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00  ................
	rel 32+8 t=1 runtime.gcbits.01+0
	rel 40+4 t=5 type..namedata.*func(string, int) float32-+0
	rel 44+4 t=-32763 type.*func(string, int) float32+0
	rel 56+8 t=1 type.string+0
	rel 64+8 t=1 type.int+0
	rel 72+8 t=1 type.float32+0
type..namedata.*main.S. SRODATA dupok size=9
	0x0000 01 07 2a 6d 61 69 6e 2e 53                       ..*main.S
type..namedata.*func(*main.S)- SRODATA dupok size=16
	0x0000 00 0e 2a 66 75 6e 63 28 2a 6d 61 69 6e 2e 53 29  ..*func(*main.S)
type.*func(*"".S) SRODATA dupok size=56
	0x0000 08 00 00 00 00 00 00 00 08 00 00 00 00 00 00 00  ................
	0x0010 7c ee 2c 84 08 08 08 36 00 00 00 00 00 00 00 00  |.,....6........
	0x0020 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00  ................
	0x0030 00 00 00 00 00 00 00 00                          ........
	rel 24+8 t=1 runtime.memequal64┬╖f+0
	rel 32+8 t=1 runtime.gcbits.01+0
	rel 40+4 t=5 type..namedata.*func(*main.S)-+0
	rel 48+8 t=1 type.func(*"".S)+0
type.func(*"".S) SRODATA dupok size=64
	0x0000 08 00 00 00 00 00 00 00 08 00 00 00 00 00 00 00  ................
	0x0010 f5 fa f4 4b 02 08 08 33 00 00 00 00 00 00 00 00  ...K...3........
	0x0020 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00  ................
	0x0030 01 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00  ................
	rel 32+8 t=1 runtime.gcbits.01+0
	rel 40+4 t=5 type..namedata.*func(*main.S)-+0
	rel 44+4 t=-32763 type.*func(*"".S)+0
	rel 56+8 t=1 type.*"".S+0
type..namedata.initAorB- SRODATA dupok size=10
	0x0000 00 08 69 6e 69 74 41 6f 72 42                    ..initAorB
type..namedata.*func()- SRODATA dupok size=9
	0x0000 00 07 2a 66 75 6e 63 28 29                       ..*func()
type.*func() SRODATA dupok size=56
	0x0000 08 00 00 00 00 00 00 00 08 00 00 00 00 00 00 00  ................
	0x0010 9b 90 75 1b 08 08 08 36 00 00 00 00 00 00 00 00  ..u....6........
	0x0020 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00  ................
	0x0030 00 00 00 00 00 00 00 00                          ........
	rel 24+8 t=1 runtime.memequal64┬╖f+0
	rel 32+8 t=1 runtime.gcbits.01+0
	rel 40+4 t=5 type..namedata.*func()-+0
	rel 48+8 t=1 type.func()+0
type.func() SRODATA dupok size=56
	0x0000 08 00 00 00 00 00 00 00 08 00 00 00 00 00 00 00  ................
	0x0010 f6 bc 82 f6 02 08 08 33 00 00 00 00 00 00 00 00  .......3........
	0x0020 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00  ................
	0x0030 00 00 00 00                                      ....
	rel 32+8 t=1 runtime.gcbits.01+0
	rel 40+4 t=5 type..namedata.*func()-+0
	rel 44+4 t=-32763 type.*func()+0
type.*"".S SRODATA dupok size=88
	0x0000 08 00 00 00 00 00 00 00 08 00 00 00 00 00 00 00  ................
	0x0010 18 b5 41 52 09 08 08 36 00 00 00 00 00 00 00 00  ..AR...6........
	0x0020 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00  ................
	0x0030 00 00 00 00 00 00 00 00 00 00 00 00 01 00 00 00  ................
	0x0040 10 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00  ................
	0x0050 00 00 00 00 00 00 00 00                          ........
	rel 24+8 t=1 runtime.memequal64┬╖f+0
	rel 32+8 t=1 runtime.gcbits.01+0
	rel 40+4 t=5 type..namedata.*main.S.+0
	rel 48+8 t=1 type."".S+0
	rel 56+4 t=5 type..importpath."".+0
	rel 72+4 t=5 type..namedata.initAorB-+0
	rel 76+4 t=26 type.func()+0
	rel 80+4 t=26 "".(*S).initAorB+0
	rel 84+4 t=26 "".(*S).initAorB+0
runtime.gcbits.02 SRODATA dupok size=1
	0x0000 02                                               .
type..namedata.I. SRODATA dupok size=3
	0x0000 01 01 49                                         ..I
type..namedata.F. SRODATA dupok size=3
	0x0000 01 01 46                                         ..F
type."".S SRODATA dupok size=144
	0x0000 10 00 00 00 00 00 00 00 10 00 00 00 00 00 00 00  ................
	0x0010 b2 68 b4 cd 07 08 08 19 00 00 00 00 00 00 00 00  .h..............
	0x0020 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00  ................
	0x0030 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00  ................
	0x0040 02 00 00 00 00 00 00 00 02 00 00 00 00 00 00 00  ................
	0x0050 00 00 00 00 00 00 00 00 40 00 00 00 00 00 00 00  ........@.......
	0x0060 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00  ................
	0x0070 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00  ................
	0x0080 00 00 00 00 00 00 00 00 10 00 00 00 00 00 00 00  ................
	rel 32+8 t=1 runtime.gcbits.02+0
	rel 40+4 t=5 type..namedata.*main.S.+0
	rel 44+4 t=5 type.*"".S+0
	rel 56+8 t=1 type."".S+96
	rel 80+4 t=5 type..importpath."".+0
	rel 96+8 t=1 type..namedata.I.+0
	rel 104+8 t=1 type.int+0
	rel 120+8 t=1 type..namedata.F.+0
	rel 128+8 t=1 type.func(string, int) float32+0
runtime.nilinterequal┬╖f SRODATA dupok size=8
	0x0000 00 00 00 00 00 00 00 00                          ........
	rel 0+8 t=1 runtime.nilinterequal+0
type..namedata.*interface {}- SRODATA dupok size=15
	0x0000 00 0d 2a 69 6e 74 65 72 66 61 63 65 20 7b 7d     ..*interface {}
type.*interface {} SRODATA dupok size=56
	0x0000 08 00 00 00 00 00 00 00 08 00 00 00 00 00 00 00  ................
	0x0010 4f 0f 96 9d 08 08 08 36 00 00 00 00 00 00 00 00  O......6........
	0x0020 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00  ................
	0x0030 00 00 00 00 00 00 00 00                          ........
	rel 24+8 t=1 runtime.memequal64┬╖f+0
	rel 32+8 t=1 runtime.gcbits.01+0
	rel 40+4 t=5 type..namedata.*interface {}-+0
	rel 48+8 t=1 type.interface {}+0
type.interface {} SRODATA dupok size=80
	0x0000 10 00 00 00 00 00 00 00 10 00 00 00 00 00 00 00  ................
	0x0010 e7 57 a0 18 02 08 08 14 00 00 00 00 00 00 00 00  .W..............
	0x0020 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00  ................
	0x0030 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00  ................
	0x0040 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00  ................
	rel 24+8 t=1 runtime.nilinterequal┬╖f+0
	rel 32+8 t=1 runtime.gcbits.02+0
	rel 40+4 t=5 type..namedata.*interface {}-+0
	rel 44+4 t=-32763 type.*interface {}+0
	rel 56+8 t=1 type.interface {}+80
type..namedata.*[]interface {}- SRODATA dupok size=17
	0x0000 00 0f 2a 5b 5d 69 6e 74 65 72 66 61 63 65 20 7b  ..*[]interface {
	0x0010 7d                                               }
type.*[]interface {} SRODATA dupok size=56
	0x0000 08 00 00 00 00 00 00 00 08 00 00 00 00 00 00 00  ................
	0x0010 f3 04 9a e7 08 08 08 36 00 00 00 00 00 00 00 00  .......6........
	0x0020 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00  ................
	0x0030 00 00 00 00 00 00 00 00                          ........
	rel 24+8 t=1 runtime.memequal64┬╖f+0
	rel 32+8 t=1 runtime.gcbits.01+0
	rel 40+4 t=5 type..namedata.*[]interface {}-+0
	rel 48+8 t=1 type.[]interface {}+0
type.[]interface {} SRODATA dupok size=56
	0x0000 18 00 00 00 00 00 00 00 08 00 00 00 00 00 00 00  ................
	0x0010 70 93 ea 2f 02 08 08 17 00 00 00 00 00 00 00 00  p../............
	0x0020 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00  ................
	0x0030 00 00 00 00 00 00 00 00                          ........
	rel 32+8 t=1 runtime.gcbits.01+0
	rel 40+4 t=5 type..namedata.*[]interface {}-+0
	rel 44+4 t=-32763 type.*[]interface {}+0
	rel 48+8 t=1 type.interface {}+0
type..importpath.fmt. SRODATA dupok size=5
	0x0000 00 03 66 6d 74                                   ..fmt
type..importpath.unsafe. SRODATA dupok size=8
	0x0000 00 06 75 6e 73 61 66 65                          ..unsafe
"".a┬╖f SRODATA dupok size=8
	0x0000 00 00 00 00 00 00 00 00                          ........
	rel 0+8 t=1 "".a+0
"".b┬╖f SRODATA dupok size=8
	0x0000 00 00 00 00 00 00 00 00                          ........
	rel 0+8 t=1 "".b+0
gclocals┬╖1a65e721a2ccc325b382662e7ffee780 SRODATA dupok size=10
	0x0000 02 00 00 00 01 00 00 00 01 00                    ..........
gclocals┬╖69c1753bd5f81501d95132d08af04464 SRODATA dupok size=8
	0x0000 02 00 00 00 00 00 00 00                          ........
"".(*S).initAorB.arginfo1 SRODATA static dupok size=3
	0x0000 00 08 ff                                         ...
"".(*S).initAorB.argliveinfo SRODATA static dupok size=2
	0x0000 00 00                                            ..
gclocals┬╖7d2d5fca80364273fb07d5820a76fef4 SRODATA dupok size=8
	0x0000 03 00 00 00 00 00 00 00                          ........
gclocals┬╖ef22736e31ca7e9ff10587cea75d17ff SRODATA dupok size=11
	0x0000 03 00 00 00 04 00 00 00 00 08 02                 ...........
"".main.stkobj SRODATA static size=40
	0x0000 02 00 00 00 00 00 00 00 e0 ff ff ff 10 00 00 00  ................
	0x0010 10 00 00 00 00 00 00 00 f0 ff ff ff 10 00 00 00  ................
	0x0020 10 00 00 00 00 00 00 00                          ........
	rel 20+4 t=5 runtime.gcbits.02+0
	rel 36+4 t=5 runtime.gcbits.02+0
gclocals┬╖2a5305abe05176240e61b8620e19a815 SRODATA dupok size=9
	0x0000 01 00 00 00 01 00 00 00 00                       .........
gclocals┬╖33cdeccccebe80329f1fdbee7f5874cb SRODATA dupok size=8
	0x0000 01 00 00 00 00 00 00 00                          ........
"".a.arginfo1 SRODATA static dupok size=9
	0x0000 fe 00 08 08 08 fd 10 08 ff                       .........
"".a.argliveinfo SRODATA static dupok size=2
	0x0000 00 00                                            ..
"".b.arginfo1 SRODATA static dupok size=9
	0x0000 fe 00 08 08 08 fd 10 08 ff                       .........
"".b.argliveinfo SRODATA static dupok size=2
	0x0000 00 00                                            ..
$f32.3fa00000 SRODATA size=4
	0x0000 00 00 a0 3f                                      ...?
$f32.40080000 SRODATA size=4
	0x0000 00 00 08 40                                      ...@
