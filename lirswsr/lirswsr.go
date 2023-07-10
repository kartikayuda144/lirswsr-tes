package lirswsr

import (
	"errors"
	"fmt"
	"log"
	"os"
	"time"

	"golang/simulator"

	"github.com/secnot/orderedmap"
)

type (
	BlockInfo struct {
		Address   int
		Operation string
		DirtyPage bool
		ColdFlag  bool
		access    int
	}
	LIRSWSR struct {
		cacheSize    int
		LIRSize      int
		HIRSize      int
		hit          int
		miss         int
		writeCount   int
		orderedStack *orderedmap.OrderedMap
		orderedList  *orderedmap.OrderedMap
		LIR          map[interface{}]int
		HIR          map[interface{}]int
		cache        map[interface{}]bool
	}
)

func NewLIRSWSR(cacheSize, HIRSize int) *LIRSWSR {
	if HIRSize > 100 || HIRSize < 0 {
		log.Fatal("HIRSize must be between 0 and 100")
	}
	LIRCapacity := (100 - HIRSize) * cacheSize / 100
	HIRCapacity := HIRSize * cacheSize / 100
	return &LIRSWSR{
		cacheSize:    cacheSize,
		LIRSize:      LIRCapacity,
		HIRSize:      HIRCapacity,
		hit:          0,
		miss:         0,
		writeCount:   0,
		orderedStack: orderedmap.NewOrderedMap(),
		orderedList:  orderedmap.NewOrderedMap(),
		LIR:          make(map[interface{}]int, LIRCapacity),
		HIR:          make(map[interface{}]int, HIRCapacity),
		cache:        make(map[interface{}]bool, cacheSize),
	}
}

func (LIRSWSRObject *LIRSWSR) Get(trace simulator.Trace) (err error) {
	block := trace.Addr
	op := trace.Op
	// if op == "W" {
	// 	LIRSWSRObject.writeCount++

	if len(LIRSWSRObject.LIR) < LIRSWSRObject.LIRSize {
		// LIR is not full; there is space in cache
		LIRSWSRObject.miss += 1
		if _, ok := LIRSWSRObject.LIR[block]; ok {
			// block is in LIR, not a miss
			LIRSWSRObject.miss -= 1
			LIRSWSRObject.hit += 1
		}
		LIRSWSRObject.addToStack(block, op)
		LIRSWSRObject.makeLIR(block)
		return nil
	}

	if _, ok := LIRSWSRObject.LIR[block]; ok {
		// hit, block is in LIR
		LIRSWSRObject.handleLIRBlock(block, op)
	} else if _, ok := LIRSWSRObject.orderedList.Get(block); ok {
		// hit, block is HIR resident
		LIRSWSRObject.handleHIRResidentBlock(block, op)
	} else {
		// miss, block is HIR non-resident
		LIRSWSRObject.handleHIRNonResidentBlock(block, op)
	}
	return nil
}

func (LIRSWSRObject *LIRSWSR) handleLIRBlock(block int, op string) (err error) {
	LIRSWSRObject.hit += 1
	key, _, ok := LIRSWSRObject.orderedStack.GetFirst()
	if !ok {
		return errors.New("orderedStack is empty")
	}
	if key.(int) == block {
		// block is in LIR and at the bottom of the stack
		// check stack
		LIRSWSRObject.condition1(false)
	}
	LIRSWSRObject.addToStack(block, op)
	LIRSWSRObject.orderedStack.Set(block, &BlockInfo{
		ColdFlag: true, // reset as cold
		access:   0,    // re-Initialize access count
	})
	LIRSWSRObject.incrementAccess(block)
	return nil
}

func (LIRSWSRObject *LIRSWSR) handleHIRResidentBlock(block int, op string) {
	LIRSWSRObject.hit += 1
	if _, ok := LIRSWSRObject.orderedStack.Get(block); ok { //if x block is in stack, move to LIR

		LIRSWSRObject.makeLIR(block)        // change x block to LIR with makeLIR
		LIRSWSRObject.removeFromList(block) //delete the x block from list q
		LIRSWSRObject.condition1(true)      //check condition 1 with value true (because HIRresident)
	} else {
		// condition2: block is not in stack, move to end of list
		LIRSWSRObject.orderedList.MoveLast(block)
		LIRSWSRObject.condition1(true)

	}
	LIRSWSRObject.addToStack(block, op) // requested x block added to top of the stack
	LIRSWSRObject.incrementAccess(block)
}

func (LIRSWSRObject *LIRSWSR) handleHIRNonResidentBlock(block int, op string) {
	LIRSWSRObject.miss += 1
	LIRSWSRObject.addToList(block, op)                      //insert the x block to the list
	if _, ok := LIRSWSRObject.orderedStack.Get(block); ok { // block is in stack, move to LIR

		LIRSWSRObject.makeLIR(block)        // change x block to LIR with makeLIR
		LIRSWSRObject.removeFromList(block) //delete the x block from list q
		LIRSWSRObject.condition3(true)      //check condition 2 with value true (because HIR non resident)

	} else {
		LIRSWSRObject.makeHIR(block)

	}
	LIRSWSRObject.addToStack(block, op) // the requested x block the top of the stack
	LIRSWSRObject.orderedStack.Set(block, &BlockInfo{
		ColdFlag: true, // reset as cold
		//	access:   0,    // re-Initialize access count
	})
	LIRSWSRObject.incrementAccess(block)
}

func (LIRSWSRObject *LIRSWSR) addToList(block int, op string) {
	if LIRSWSRObject.orderedList.Len() == LIRSWSRObject.HIRSize {
		LIRSWSRObject.orderedList.PopFirst()
	}

	if op == "W" {
		LIRSWSRObject.writeCount++ // Increment the write count
		LIRSWSRObject.orderedList.Set(block, &BlockInfo{
			Address:   block,
			Operation: op,
			DirtyPage: true, // Set as dirty page
			ColdFlag:  true, // Set as cold
			access:    0,    // Initialize access count
		})
	} else {
		LIRSWSRObject.orderedList.Set(block, &BlockInfo{
			Address:   block,
			Operation: op,
			DirtyPage: false, // Set as clean page
			ColdFlag:  true,  // Set as cold
			access:    0,     // Initialize access count
		})
	}
}

func (LIRSWSRObject *LIRSWSR) addToStack(block int, op string) {
	if _, ok := LIRSWSRObject.orderedStack.Get(block); ok {
		LIRSWSRObject.orderedStack.MoveLast(block)
		return

	}
	// Check if the block is introduced for write request for the first time or if it is a dirty page
	blockInfo := &BlockInfo{
		Address:   block,
		Operation: op,
		ColdFlag:  true, // Set as cold
		access:    0,    // Initialize access count
	}

	if op == "W" {
		blockInfo.DirtyPage = true // Set DirtyPage to true for write operation
		LIRSWSRObject.writeCount++ // Increment the write count
	} else {
		blockInfo.DirtyPage = false // Set DirtyPage to false for other operations
	}

	LIRSWSRObject.orderedStack.Set(block, blockInfo)
}

func (LIRSWSRObject *LIRSWSR) removeFromList(block int) {
	LIRSWSRObject.orderedList.Delete(block)
}

func (LIRSWSRObject *LIRSWSR) makeLIR(block int) {
	delete(LIRSWSRObject.HIR, block)
	LIRSWSRObject.LIR[block] = 1
}

func (LIRSWSRObject *LIRSWSR) makeHIR(block int) {
	delete(LIRSWSRObject.LIR, block)
	LIRSWSRObject.HIR[block] = 1
}

func (LIRSWSRObject *LIRSWSR) condition1(removeLIR bool) (err error) {
	key, _, ok := LIRSWSRObject.orderedStack.PopFirst()
	if !ok {
		return errors.New("orderedStack is empty")
	}

	if removeLIR {
		block := key.(int)
		if LIRSWSRObject.isBlockColdDirty(block) || LIRSWSRObject.isColdFlag(block) {
			// Clean page or cold-dirty page moves to the end of the list Q
			LIRSWSRObject.makeHIR(block)
			LIRSWSRObject.orderedList.Set(block, 1)
			LIRSWSRObject.orderedList.MoveLast(block)
		} else {
			//Not-cold dirty page in the bottom of the stack S is moved to the top with Cold flag set
			LIRSWSRObject.miss += 1
			LIRSWSRObject.orderedStack.Set(block, &BlockInfo{
				ColdFlag: true, // Set as cold
				access:   0,    // Initialize access count
			})
			LIRSWSRObject.orderedStack.MoveLast(block)
		}
	}

	LIRSWSRObject.stackPruning()
	return nil
}

func (LIRSWSRObject *LIRSWSR) condition3(removeLIR bool) (err error) {
	key, _, ok := LIRSWSRObject.orderedStack.PopFirst()
	if !ok {
		return errors.New("orderedStack is empty")
	}

	if removeLIR {
		block := key.(int)
		if LIRSWSRObject.isBlockColdDirty(block) || LIRSWSRObject.isColdFlag(block) {
			// Clean page or cold-dirty page moves to the end of the list Q
			LIRSWSRObject.makeHIR(block)
			LIRSWSRObject.orderedList.Set(block, 1)
			LIRSWSRObject.orderedList.MoveLast(block)
		} else {
			// Not-cold dirty page in the bottom of the stack S is moved to the top with Cold flag set
			LIRSWSRObject.miss += 1
			LIRSWSRObject.orderedStack.Set(block, &BlockInfo{
				ColdFlag: true, // Set as cold
				access:   0,    // Initialize access count
			})
			LIRSWSRObject.orderedStack.MoveLast(block)
		}
	}

	LIRSWSRObject.stackPruning()
	return nil
}

func (LIRSWSRObject *LIRSWSR) stackPruning() {
	iter := LIRSWSRObject.orderedStack.Iter()
	for k, _, ok := iter.Next(); ok; k, _, ok = iter.Next() {
		if _, ok := LIRSWSRObject.LIR[k]; ok {
			break
		}
		LIRSWSRObject.orderedStack.PopFirst()
	}
}

func (LIRSWSRObject *LIRSWSR) isColdFlag(block int) bool {
	if blockInfo, ok := LIRSWSRObject.orderedStack.Get(block); ok {
		accessCount := blockInfo.(*BlockInfo).access
		return accessCount < 2
	}
	return false
}
func (LIRSWSRObject *LIRSWSR) isDirtyPage(block int) bool {
	if blockInfo, ok := LIRSWSRObject.orderedStack.Get(block); ok {
		return blockInfo.(*BlockInfo).DirtyPage
	}
	return false
}

func (LIRSWSRObject *LIRSWSR) isBlockColdDirty(block int) bool {
	if blockInfo, ok := LIRSWSRObject.orderedStack.Get(block); ok {
		return blockInfo.(*BlockInfo).ColdFlag && blockInfo.(*BlockInfo).DirtyPage
	}
	return false
}

func (LIRSWSRObject *LIRSWSR) incrementAccess(block int) {
	if blockInfo, ok := LIRSWSRObject.orderedStack.Get(block); ok {
		blockInfo.(*BlockInfo).access++
	}
}

func (LIRSWSRObject *LIRSWSR) PrintToFile(file *os.File, start time.Time) (err error) {
	duration := time.Since(start)
	hitRatio := 100 * float32(float32(LIRSWSRObject.hit)/float32(LIRSWSRObject.hit+LIRSWSRObject.miss))
	result := fmt.Sprintf(`_______________________________________________________
LIRSWSRmendekatibenar fungsi checker clean page bottom stack move end of the list dihilangkan
cache size : %v
cache hit : %v
cache miss : %v
hit ratio : %v
list size : %v
stack size : %v
lir capacity: %v
hir capacity: %v
write count : %v
duration : %v
!LIRSWSR|%v|%v|%v
`, LIRSWSRObject.cacheSize, LIRSWSRObject.hit, LIRSWSRObject.miss, hitRatio, LIRSWSRObject.orderedList.Len(), LIRSWSRObject.orderedStack.Len(), LIRSWSRObject.LIRSize, LIRSWSRObject.HIRSize, LIRSWSRObject.writeCount, duration.Seconds(), LIRSWSRObject.cacheSize, LIRSWSRObject.hit, LIRSWSRObject.hit+LIRSWSRObject.miss)
	_, err = file.WriteString(result)
	return err
}
