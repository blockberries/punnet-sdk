package types

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

// Benchmark helpers to create test SignDocs of various sizes

// createSmallSignDoc creates a SignDoc with 1 message and minimal fields.
// Target: small transaction with single operation.
func createSmallSignDoc() *SignDoc {
	sd := NewSignDoc("test-chain", 1, "alice", 1, "")
	sd.AddMessage("/punnet.bank.v1.MsgSend", json.RawMessage(`{"from":"alice","to":"bob","amount":"100"}`))
	return sd
}

// createMediumSignDoc creates a SignDoc with 3 messages and typical fields.
// Target: typical multi-message transaction.
func createMediumSignDoc() *SignDoc {
	sd := NewSignDoc("mainnet-production", 12345, "cosmos1abc...xyz", 12345, "batch transfer")
	sd.AddMessage("/punnet.bank.v1.MsgSend", json.RawMessage(`{"from":"cosmos1abc...xyz","to":"cosmos1def...uvw","amount":{"denom":"stake","amount":"1000000"}}`))
	sd.AddMessage("/punnet.bank.v1.MsgSend", json.RawMessage(`{"from":"cosmos1abc...xyz","to":"cosmos1ghi...rst","amount":{"denom":"stake","amount":"500000"}}`))
	sd.AddMessage("/punnet.staking.v1.MsgDelegate", json.RawMessage(`{"delegator":"cosmos1abc...xyz","validator":"cosmosvaloper1xyz...","amount":{"denom":"stake","amount":"2000000"}}`))
	return sd
}

// createLargeSignDoc creates a SignDoc with 10 messages and max memo.
// Target: stress test with maximum practical transaction size.
func createLargeSignDoc() *SignDoc {
	// Max memo is 512 bytes
	memo := strings.Repeat("x", 512)
	sd := NewSignDoc("mainnet-production-chain-id-long-name", 999999999, "cosmos1verylongaddresshere000000000000000000abc", 999999999, memo)

	// Add 10 messages with substantial data
	for i := 0; i < 10; i++ {
		// Create a message with ~1KB of data
		data := fmt.Sprintf(`{"from":"cosmos1sender%d","to":"cosmos1receiver%d","amount":{"denom":"ustake","amount":"%d"},"metadata":{"memo":"%s","tags":["%s","%s","%s"]}}`,
			i, i, i*1000000,
			strings.Repeat("m", 200),
			strings.Repeat("t", 100),
			strings.Repeat("a", 100),
			strings.Repeat("g", 100))
		sd.AddMessage(fmt.Sprintf("/punnet.bank.v1.MsgSend%d", i), json.RawMessage(data))
	}
	return sd
}

// ============================================================================
// Serialization Benchmarks
// ============================================================================

func BenchmarkSignDocToJSON_Small(b *testing.B) {
	sd := createSmallSignDoc()
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, err := sd.ToJSON()
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkSignDocToJSON_Medium(b *testing.B) {
	sd := createMediumSignDoc()
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, err := sd.ToJSON()
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkSignDocToJSON_Large(b *testing.B) {
	sd := createLargeSignDoc()
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, err := sd.ToJSON()
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkSignDocGetSignBytes_Small(b *testing.B) {
	sd := createSmallSignDoc()
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, err := sd.GetSignBytes()
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkSignDocGetSignBytes_Medium(b *testing.B) {
	sd := createMediumSignDoc()
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, err := sd.GetSignBytes()
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkSignDocGetSignBytes_Large(b *testing.B) {
	sd := createLargeSignDoc()
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, err := sd.GetSignBytes()
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkParseSignDoc measures JSON deserialization performance.
func BenchmarkParseSignDoc_Small(b *testing.B) {
	sd := createSmallSignDoc()
	jsonBytes, err := sd.ToJSON()
	if err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, err := ParseSignDoc(jsonBytes)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkParseSignDoc_Medium(b *testing.B) {
	sd := createMediumSignDoc()
	jsonBytes, err := sd.ToJSON()
	if err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, err := ParseSignDoc(jsonBytes)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkParseSignDoc_Large(b *testing.B) {
	sd := createLargeSignDoc()
	jsonBytes, err := sd.ToJSON()
	if err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, err := ParseSignDoc(jsonBytes)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkSignDocValidateBasic measures validation performance.
func BenchmarkSignDocValidateBasic_Small(b *testing.B) {
	sd := createSmallSignDoc()
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		err := sd.ValidateBasic()
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkSignDocValidateBasic_Medium(b *testing.B) {
	sd := createMediumSignDoc()
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		err := sd.ValidateBasic()
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkSignDocValidateBasic_Large(b *testing.B) {
	sd := createLargeSignDoc()
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		err := sd.ValidateBasic()
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkSignDocEquals measures equality comparison performance.
func BenchmarkSignDocEquals_Small(b *testing.B) {
	sd1 := createSmallSignDoc()
	sd2 := createSmallSignDoc()
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = sd1.Equals(sd2)
	}
}

func BenchmarkSignDocEquals_Medium(b *testing.B) {
	sd1 := createMediumSignDoc()
	sd2 := createMediumSignDoc()
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = sd1.Equals(sd2)
	}
}

func BenchmarkSignDocEquals_Large(b *testing.B) {
	sd1 := createLargeSignDoc()
	sd2 := createLargeSignDoc()
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = sd1.Equals(sd2)
	}
}

// ============================================================================
// Message Count Scaling Benchmarks
// ============================================================================

// BenchmarkSignDocToJSON_MessageScaling measures how performance scales with message count.
func BenchmarkSignDocToJSON_MessageScaling(b *testing.B) {
	messageCounts := []int{1, 5, 10, 25, 50, 100, 256}

	for _, count := range messageCounts {
		b.Run(fmt.Sprintf("Messages_%d", count), func(b *testing.B) {
			sd := NewSignDoc("test-chain", 1, "alice", 1, "")
			for i := 0; i < count; i++ {
				sd.AddMessage(fmt.Sprintf("/msg/%d", i), json.RawMessage(`{"key":"value"}`))
			}
			b.ResetTimer()
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				_, err := sd.ToJSON()
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkSignDocGetSignBytes_MessageScaling measures hash performance scaling.
func BenchmarkSignDocGetSignBytes_MessageScaling(b *testing.B) {
	messageCounts := []int{1, 5, 10, 25, 50, 100, 256}

	for _, count := range messageCounts {
		b.Run(fmt.Sprintf("Messages_%d", count), func(b *testing.B) {
			sd := NewSignDoc("test-chain", 1, "alice", 1, "")
			for i := 0; i < count; i++ {
				sd.AddMessage(fmt.Sprintf("/msg/%d", i), json.RawMessage(`{"key":"value"}`))
			}
			b.ResetTimer()
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				_, err := sd.GetSignBytes()
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// ============================================================================
// Message Data Size Scaling Benchmarks
// ============================================================================

// BenchmarkSignDocToJSON_DataSizeScaling measures how performance scales with message data size.
func BenchmarkSignDocToJSON_DataSizeScaling(b *testing.B) {
	dataSizes := []int{100, 1024, 4096, 16384, 65536} // 100B, 1KB, 4KB, 16KB, 64KB (max)

	for _, size := range dataSizes {
		b.Run(fmt.Sprintf("DataSize_%d", size), func(b *testing.B) {
			sd := NewSignDoc("test-chain", 1, "alice", 1, "")
			data := fmt.Sprintf(`{"payload":"%s"}`, strings.Repeat("x", size-15))
			sd.AddMessage("/msg/large", json.RawMessage(data))
			b.ResetTimer()
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				_, err := sd.ToJSON()
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
