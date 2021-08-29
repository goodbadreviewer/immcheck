package immcheck_test

import (
	"fmt"
	"hash/crc32"
	"hash/maphash"
	"math/rand"
	"testing"

	"github.com/cespare/xxhash/v2"
	"github.com/goodbadreviewer/immcheck"
)

var sizeOfByteSlice = []int{
	16 * 1024,
}

var percentOfMutations = []int{
	0, 1,
}

var countOfTransactions = []int{
	8,
}

var sizeOfTxContext = []int{
	8, 1024,
}

var count = 0

func BenchmarkImmcheckBytes(b *testing.B) {
	for _, targetSize := range sizeOfByteSlice {
		for _, mutationPercent := range percentOfMutations {
			benchName := fmt.Sprintf("[%v]byte;muts(%v%%)", targetSize, mutationPercent)
			b.Run(benchName, func(b *testing.B) {
				localRand := rand.New(rand.NewSource(rand.Int63()))
				count = 0

				targetObjects := make([][]byte, b.N)
				for i := 0; i < b.N; i++ {
					targetObjects[i] = make([]byte, targetSize)
					localRand.Read(targetObjects[i])
				}

				runBytesBenchmark(
					b, targetObjects,
					immcheck.Options{Flags: immcheck.SkipOriginCapturing | immcheck.SkipLoggingOnMutation},
					mutationPercent,
				)
			})
		}
	}
}

func runBytesBenchmark(
	b *testing.B,
	targetObjects [][]byte,
	options immcheck.Options,
	mutationPercent int) {
	b.Helper()
	b.ResetTimer()
	b.ReportAllocs()
	original := immcheck.NewValueSnapshot()
	other := immcheck.NewValueSnapshot()
	for i := 0; i < b.N; i++ {
		snapshot := immcheck.CaptureSnapshotWithOptions(&targetObjects[i], original, options)
		rndValue := rand.Intn(100)
		if rndValue < mutationPercent {
			targetObjects[i][0] = byte(rndValue)
		}
		otherSnapshot := immcheck.CaptureSnapshotWithOptions(&targetObjects[i], other, options)
		err := snapshot.CheckImmutabilityAgainst(otherSnapshot)
		if err != nil {
			count++
		}
	}
	b.ReportMetric(float64(count), "muts")
}

func BenchmarkImmcheckTransactions(b *testing.B) {
	for _, txCnt := range countOfTransactions {
		for _, ctxSize := range sizeOfTxContext {
			for _, mutationPercent := range percentOfMutations {
				benchName := fmt.Sprintf("[%v]txs(%v);muts(%v%%)", txCnt, ctxSize, mutationPercent)
				b.Run(benchName, func(b *testing.B) {
					localRand := rand.New(rand.NewSource(rand.Int63()))
					count = 0

					targetObjects := make([][]*Transaction, b.N)
					for i := 0; i < b.N; i++ {
						targetObjects[i] = make([]*Transaction, txCnt)
						for j := 0; j < txCnt; j++ {
							targetObjects[i][j] = GenerateTransaction(localRand, ctxSize)
						}
					}

					runTransactionsBenchmark(
						b, targetObjects,
						immcheck.Options{Flags: immcheck.SkipOriginCapturing | immcheck.SkipLoggingOnMutation},
						mutationPercent,
					)
				})
			}
		}
	}
}

func BenchmarkHash(b *testing.B) {
	for s := 4; s < 1024; s *= 2 {
		b.Run(fmt.Sprintf("crc32-%v", s), func(b *testing.B) {
			target := make([]byte, s+b.N)
			rand.Read(target)
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				count += int(crc32.ChecksumIEEE(target[i : i+s]))
			}
		})
		b.Run(fmt.Sprintf("xxhash-%v", s), func(b *testing.B) {
			target := make([]byte, s+b.N)
			rand.Read(target)
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				count += int(xxhash.Sum64(target[i : i+s]))
			}
		})
		b.Run(fmt.Sprintf("maphash-%v", s), func(b *testing.B) {
			target := make([]byte, s+b.N)
			rand.Read(target)
			b.ResetTimer()
			hash := maphash.Hash{}
			for i := 0; i < b.N; i++ {
				hash.Reset()
				_, _ = hash.Write(target[i : i+s])
				count += int(hash.Sum64())
			}
		})
	}
}

func runTransactionsBenchmark(
	b *testing.B,
	targetObjects [][]*Transaction,
	options immcheck.Options,
	mutationPercent int) {
	b.ResetTimer()
	b.ReportAllocs()
	original := immcheck.NewValueSnapshot()
	other := immcheck.NewValueSnapshot()
	for i := 0; i < b.N; i++ {
		snapshot := immcheck.CaptureSnapshotWithOptions(&targetObjects[i], original, options)
		rndValue := rand.Intn(100)
		if rndValue < mutationPercent {
			targetObjects[i][0].Amount.Amount = Amount(rndValue)
		}
		otherSnapshot := immcheck.CaptureSnapshotWithOptions(&targetObjects[i], other, options)
		err := snapshot.CheckImmutabilityAgainst(otherSnapshot)
		if err != nil {
			count++
		}
	}
	b.ReportMetric(float64(count), "muts")
}

type CurrencyCode int

const (
	USD CurrencyCode = iota
	EUR
)

type Amount int64

type Currency struct {
	Code     CurrencyCode
	Fraction uint64
}

var Currencies = map[CurrencyCode]Currency{
	USD: {Code: USD, Fraction: 2},
	EUR: {Code: EUR, Fraction: 2},
}

type Money struct {
	Currency Currency
	Amount   Amount
}

type AccountType int

const (
	Credit AccountType = iota
	Debit
)

type Account struct {
	Address [16]byte
	Type    AccountType
}

type AccountState struct {
	Account Account
	Balance Money
}

type StateSnapshot struct {
	SrcState AccountState
	DstState AccountState
}

type Transaction struct {
	Src         Account
	Dst         Account
	Amount      Money
	StateBefore *StateSnapshot
	StateAfter  *StateSnapshot
	TxContext   []string
	Attachments map[string]interface{}
}

func GenerateTransaction(rnd *rand.Rand, contextSize int) *Transaction {
	currencyCode := CurrencyCode(rnd.Intn(2))
	targetCurrency := Currencies[currencyCode]

	srcAddress := [16]byte{}
	rnd.Read(srcAddress[:])
	srcAccount := Account{
		Address: srcAddress,
		Type:    AccountType(rnd.Intn(2)),
	}

	dstAddress := [16]byte{}
	rnd.Read(dstAddress[:])
	dstAccount := Account{
		Address: dstAddress,
		Type:    AccountType(rnd.Intn(2)),
	}

	transferAmount := Money{
		Currency: targetCurrency,
		Amount:   Amount(int64(rnd.Uint32()) + 1),
	}

	before := &StateSnapshot{
		SrcState: AccountState{
			Account: srcAccount,
			Balance: Money{
				Currency: targetCurrency,
				Amount:   Amount(int64(transferAmount.Amount) + int64(rnd.Uint32())),
			},
		},
		DstState: AccountState{
			Account: dstAccount,
			Balance: Money{
				Currency: targetCurrency,
				Amount:   Amount(int64(rnd.Uint32())),
			},
		},
	}

	after := &StateSnapshot{
		SrcState: AccountState{
			Account: srcAccount,
			Balance: Money{
				Currency: targetCurrency,
				Amount:   Amount(int64(before.SrcState.Balance.Amount) - int64(transferAmount.Amount)),
			},
		},
		DstState: AccountState{
			Account: dstAccount,
			Balance: Money{
				Currency: targetCurrency,
				Amount:   Amount(int64(before.DstState.Balance.Amount) + int64(transferAmount.Amount)),
			},
		},
	}

	txContext := make([]string, contextSize)
	for i := 0; i < contextSize; i++ {
		value := make([]byte, rand.Intn(4096))
		rnd.Read(value)
		txContext[i] = string(value)
	}

	return &Transaction{
		Src:         srcAccount,
		Dst:         dstAccount,
		Amount:      transferAmount,
		StateBefore: before,
		StateAfter:  after,
		TxContext:   txContext,
		Attachments: map[string]interface{}{
			"bank": struct {
				Name        string
				Reliability uint
			}{
				"TestBank",
				1,
			},
			"certificate": []byte{1, 2, 3},
		},
	}
}
