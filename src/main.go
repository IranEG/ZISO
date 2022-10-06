package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/pborman/getopt/v2"
	"github.com/pierrec/lz4/v4"
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

func open_input_output(fname_in string, fname_out string) (*os.File, *os.File) {
	fin, err := os.Open(fname_in)
	if err != nil {
		panic(err)
	}

	fout, err := os.Create(fname_out)
	if err != nil {
		panic(err)
	}

	return fin, fout
}

func generate_zso_header(magic int, header_size int, total_bytes int64, block_size int, ver int, align int) []byte {
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

func pack(int_byte int32) []byte {
	type packet struct {
		_int_byte uint32
	}

	dataIn := packet{_int_byte: uint32(int_byte)}
	buf := new(bytes.Buffer)

	binary.Write(buf, binary.LittleEndian, dataIn)

	return buf.Bytes()
}

func show_comp_info(fname_in string, fname_out string, total_bytes int64, block_size int, align int, level int) {
	fmt.Printf("Compress '%s' to '%s'\n", fname_in, fname_out)
	fmt.Printf("Total File Size: %d bytes\n", total_bytes)
	fmt.Printf("Block size: 	 %d bytes\n", block_size)
	fmt.Printf("Index Align: 	 %d\n", (1 << align))
	fmt.Printf("Compress level:  %d\n", level)

}

func set_align(fout *os.File, write_pos int64, align int) int64 {
	if (write_pos % (1 << align)) == 0 {
		align_len := (1 << align) - write_pos%(1<<align)
		padding := make([]byte, align_len)
		for j := range padding {
			padding[j] = DEFAULT_PADDING
		}
		fout.Write(padding)
		var pad = make([]byte, align_len)
		for i := range pad {
			pad[i] = DEFAULT_PADDING
		}
		fout.Write(pad)
		write_pos += align_len
	}

	return write_pos
}

func compress_zso(fname_in string, fname_out string, level int) {
	fin, fout := open_input_output(fname_in, fname_out)

	file_stat, err := fin.Stat()
	if err != nil {
		panic(err)
	}
	total_bytes := file_stat.Size()

	magic, header_size, block_size, ver, align := ZISO_MAGIC, 0x18, 0x800, 1, DEFAULT_ALIGN

	align = int(total_bytes) / 0x80000000

	header := generate_zso_header(magic, header_size, total_bytes, block_size, ver, align)

	fout.Write(header)

	total_block := total_bytes / int64(block_size)

	var index_buf = make([]int, total_block+1)
	var blank_bytes = make([]byte, len(index_buf)*4)

	fout.Write(blank_bytes)

	show_comp_info(fname_in, fname_out, total_bytes, block_size, align, level)

	write_pos, err := fout.Seek(0, io.SeekCurrent)
	if err != nil {
		panic(err)
	}

	percent_period := float64(total_block) / 100
	percent_cnt := int64(0)

	var block int64 = 0

	fmt.Printf("total_block: %d\n", total_block)
	fmt.Printf("percent_period: %f\n", percent_period)
	fmt.Printf("percent_cnt: %d\n", percent_cnt)

	iso_data := make([]byte, block_size)

	var c lz4.CompressorHC
	c.Level = lz4.CompressionLevel(level)

	for block < total_block {
		_, err := fin.Read(iso_data)
		if err != nil && err != io.EOF {
			panic(err)
		}

		zso_data := make([]byte, block_size)

		n, err := c.CompressBlock(iso_data, zso_data)

		if err != nil {
			fmt.Println("Compressed data does not fit in zso_data")
			panic(err)
		}

		write_pos = set_align(fout, write_pos, align)
		index_buf[block] = int(write_pos) >> align

		if n == 0 {
			zso_data = iso_data
			index_buf[block] |= 0x80000000
		} else if (index_buf[block] & 0x80000000) > 0 {
			fmt.Printf("Align error, you have to increase align by 1 or CFW won't be able to read offset above 2 ** 31 bytes")
			os.Exit(1)
		} else {
			zso_data = zso_data[:n]
		}

		fout.Write(zso_data)
		write_pos += int64(len(zso_data))
		block++
	}

	index_buf[block] = int(write_pos) >> int(align)
	fout.Seek(int64(len(header)), 0)

	for i := range index_buf {
		idx := pack(int32(index_buf[i]))
		fout.Write(idx)
	}

	fin.Close()
	fout.Close()

}

func decompress_zso(fname_in string, fname_out string)

func main() {
	fmt.Printf("ziso-go %s by %s\n", __version__, __author__)
	level, fname_in, fname_out := parse_args()

	if level == 0 {
		decompress_zso(fname_in, fname_out)
	} else {
		compress_zso(fname_in, fname_out, level)
	}
}
