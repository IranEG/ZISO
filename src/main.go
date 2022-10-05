package main

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/pborman/getopt/v2"
	"github.com/pierrec/lz4"
)

const __version__ = "1.0"
const __author__ = "IranEG"

const ZISO_MAGIC = 0x4F53495A

var DEFAULT_ALIGN int
var COMPRESS_THRESHOLD int
var DEFAULT_PADDING byte
var MP bool

const MP_NR = 1024 * 16

func parse_args() (int, string, string) {
	var fname_in string
	var fname_out string

	level := *getopt.IntLong("compress", 'c', 1, "value: 1-9 compress ISO to ZSO, use any non-zero number it has no effect\n              0 decompress ZSO to ISO")
	MP = *getopt.BoolLong("multiproc", 'm', "Use multiprocessing acceleration for compressing")
	COMPRESS_THRESHOLD = *getopt.IntLong("threshold", 't', 100, "percent Compression Threshold (1-100)")
	DEFAULT_ALIGN = *getopt.IntLong("align", 'a', 0, "value: Padding alignment 0=small/slow 6=fast/large")
	padding := *getopt.StringLong("padding", 'p', "X", "value: Padding byte")
	help := getopt.BoolLong("help", 'h', "Help")

	getopt.ParseV2()

	args := getopt.NArgs()

	if len(os.Args) < 2 {
		getopt.Usage()
		os.Exit(-1)
	}

	if *help {
		getopt.Usage()
		os.Exit(0)
	}

	if (level > 9) && (level < 1) {
		fmt.Printf("!!!Error: out of bounds value for compress!!!\n\n")
		getopt.Usage()
		os.Exit(1)
	}

	if (COMPRESS_THRESHOLD > 100) && (COMPRESS_THRESHOLD < 0) {
		fmt.Printf("!!!Error: out of bounds value for threshold!!!\n\n")
		getopt.Usage()
		os.Exit(1)
	}

	if (args < 1) || (args > 2) {
		fmt.Printf("!!!Error: Invalid amount of input and output file parameters!!!\n\n")
		getopt.Usage()
		os.Exit(1)
	}

	if len(padding) == 1 {
		DEFAULT_PADDING = ([]byte(padding))[0]
	} else {
		fmt.Printf("!!!Error: Invalid padding character!!!\n\n")
		getopt.Usage()
		os.Exit(1)
	}

	if args == 1 {
		if strings.Contains(getopt.Arg(0), ".iso") {
			fname_in = getopt.Arg(0)
			fname_out = strings.Replace(getopt.Arg(0), ".iso", ".zso", 1)
		} else {
			fmt.Printf("!!!Error: Invalid file extension!!!\n\n")
			getopt.Usage()
			os.Exit(1)
		}
	}

	if args == 2 {
		if strings.Contains(getopt.Arg(0), ".iso") && (strings.Contains(getopt.Arg(1), ".zso")) {
			fname_in = getopt.Arg(0)
			fname_out = getopt.Arg(1)
		} else {
			fmt.Printf("!!!Error: Invalid file extension!!!\n\n")
			getopt.Usage()
			os.Exit(1)
		}
	}

	return level, fname_in, fname_out
}

func generate_zso_header(magic int, header_size int, total_bytes int64, block_size int, ver int, align int64) []byte {
	type packet struct {
		_magic       uint32
		_header_size uint32
		_total_bytes uint64
		_block_size  uint32
		_ver         byte
		_align       byte
		_pad_byte1   byte
		_pad_byte2   byte
	}

	dataIn := packet{_magic: uint32(magic), _header_size: uint32(header_size), _total_bytes: uint64(total_bytes), _block_size: uint32(block_size), _ver: byte(ver), _align: byte(align)}
	buf := new(bytes.Buffer)

	binary.Write(buf, binary.LittleEndian, dataIn)

	return buf.Bytes()
}

func show_comp_info(fname_in string, fname_out string, total_bytes int64, block_size int, align int64, level int) {
	fmt.Printf("Compress '%s' to '%s'\n", fname_in, fname_out)
	fmt.Printf("Total File Size: %d bytes\n", total_bytes)
	fmt.Printf("Block size: 	 %d bytes\n", block_size)
	fmt.Printf("Index Align: 	 %d\n", (1 << align))
	fmt.Printf("Compress level:  %d\n", level)

}

func compress_zso(fname_in string, fname_out string, level int) {
	fin, err := os.Open(fname_in)
	if err != nil {
		panic(err)
	}

	defer func() {
		if err := fin.Close(); err != nil {
			panic(err)
		}
	}()

	fout, err := os.Create(fname_out)
	if err != nil {
		panic(err)
	}

	defer func() {
		if err := fout.Close(); err != nil {
			panic(err)
		}
	}()

	file_stat, err := fin.Stat()
	if err != nil {
		panic(err)
	}
	total_bytes := file_stat.Size()

	fmt.Printf("File Size: %d bytes\n", total_bytes)

	magic, header_size, block_size, ver, align := ZISO_MAGIC, 0x18, 0x800, 1, int64(DEFAULT_ALIGN)

	align = total_bytes / 0x80000000

	header := generate_zso_header(magic, header_size, total_bytes, block_size, ver, align)

	fout.Write(header)

	total_block := total_bytes / int64(block_size)

	var index_buf = make([]int, total_block+1)

	for i, s := range index_buf {
		fout.Write([]byte{0x00, 0x00, 0x00, 0x00})
		fmt.Printf("%d %d\n", i, s)
	}

	show_comp_info(fname_in, fname_out, total_bytes, block_size, align, level)

}

func decompress_zso(fname_in, fname_out) {

}

func main() {
	fmt.Printf("ziso-go %s by %s\n", __version__, __author__)
	level, fname_in, fname_out := parse_args()

	if level == 0 {
		decompress_zso(fname_in, fname_out)
	} else {
		compress_zso(fname_in, fname_out, level)
	}

	fmt.Println(level)
	fmt.Println(fname_in)
	fmt.Println(fname_out)
	fmt.Println(*MP)
	fmt.Println(*COMPRESS_THRESHOLD)
	fmt.Println(*DEFAULT_ALIGN)
	fmt.Println(DEFAULT_PADDING)
	fmt.Printf("0x%X\n", ZISO_MAGIC)

	//compress(os.Args[1], os.Args[2])
	//decompress(os.Args[2], "decompress.txt")
}

// ====================================================================================//
func compress(inputFile, outputFile string) {
	// open input file
	fin, err := os.Open(inputFile)
	if err != nil {
		panic(err)
	}
	defer func() {
		if err := fin.Close(); err != nil {
			panic(err)
		}
	}()
	// make a read buffer
	r := bufio.NewReader(fin)

	// open output file
	fout, err := os.Create(outputFile)
	if err != nil {
		panic(err)
	}
	defer func() {
		if err := fout.Close(); err != nil {
			panic(err)
		}
	}()
	// make an lz4 write buffer
	w := lz4.NewWriter(fout)

	// make a buffer to keep chunks that are read
	buf := make([]byte, 1024)
	for {
		// read a chunk
		n, err := r.Read(buf)
		if err != nil && err != io.EOF {
			panic(err)
		}
		if n == 0 {
			break
		}

		// write a chunk
		if _, err := w.Write(buf[:n]); err != nil {
			panic(err)
		}
	}

	if err = w.Flush(); err != nil {
		panic(err)
	}
}

func decompress(inputFile, outputFile string) {
	// open input file
	fin, err := os.Open(inputFile)
	if err != nil {
		panic(err)
	}
	defer func() {
		if err := fin.Close(); err != nil {
			panic(err)
		}
	}()

	// make an lz4 read buffer
	r := lz4.NewReader(fin)

	// open output file
	fout, err := os.Create(outputFile)
	if err != nil {
		panic(err)
	}
	defer func() {
		if err := fout.Close(); err != nil {
			panic(err)
		}
	}()

	// make a write buffer
	w := bufio.NewWriter(fout)

	// make a buffer to keep chunks that are read
	buf := make([]byte, 1024)
	for {
		// read a chunk
		n, err := r.Read(buf)
		if err != nil && err != io.EOF {
			panic(err)
		}
		if n == 0 {
			break
		}

		// write a chunk
		if _, err := w.Write(buf[:n]); err != nil {
			panic(err)
		}
	}

	if err = w.Flush(); err != nil {
		panic(err)
	}
}
