package lirs

import (
	"errors"
	"fmt"
	"log"
	"os"
	"time"

	"golang/simulator"

	"github.com/secnot/orderedmap"
)

type LIRS struct {
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

func NewLIRS(cacheSize, HIRSize int) *LIRS {
	if HIRSize > 100 || HIRSize < 0 {
		log.Fatal("HIRSize must be between 0 and 100")
	}
	LIRCapacity := (100 - HIRSize) * cacheSize / 100
	HIRCapacity := HIRSize * cacheSize / 100
	return &LIRS{
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

func (LIRSObject *LIRS) Get(trace simulator.Trace) (err error) {
	block := trace.Addr
	op := trace.Op
	if op == "W" {
		LIRSObject.writeCount++
	}

	if len(LIRSObject.LIR) < LIRSObject.LIRSize {
		// LIR is not full; there is space in cache
		LIRSObject.miss += 1
		if _, ok := LIRSObject.LIR[block]; ok {
			// block is in LIR, not a miss
			LIRSObject.miss -= 1
			LIRSObject.hit += 1
		}
		LIRSObject.addToStack(block)
		LIRSObject.makeLIR(block)
		return nil
	}

	if _, ok := LIRSObject.LIR[block]; ok {
		// hit, block is in LIR
		LIRSObject.handleLIRBlock(block)
	} else if _, ok := LIRSObject.orderedList.Get(block); ok {
		// hit, block is HIR resident
		LIRSObject.handleHIRResidentBlock(block)
	} else {
		// miss, blok is HIR non resident
		LIRSObject.handleHIRNonResidentBlock(block)
	}
	return nil
}

func (LIRSObject *LIRS) handleLIRBlock(block int) (err error) {
	LIRSObject.hit += 1
	key, _, ok := LIRSObject.orderedStack.GetFirst()
	if !ok {
		return errors.New("orderedStack is empty")
	}
	if key.(int) == block { // block x is in LIR and at the bottom of the stack

		LIRSObject.condition1(false) // do stack pruning
	}
	LIRSObject.addToStack(block)
	return nil
}

func (LIRSObject *LIRS) handleHIRResidentBlock(block int) {
	LIRSObject.hit += 1
	if _, ok := LIRSObject.orderedStack.Get(block); ok {

		LIRSObject.makeLIR(block)        // block x is in stack, move to LIR
		LIRSObject.removeFromList(block) // block x is in stack, remove from List Q
		LIRSObject.condition1(true)
	} else {
		// condition2: block is not in stack, move to end of list
		LIRSObject.orderedList.MoveLast(block)
	}
	LIRSObject.addToStack(block)
}

func (LIRSObject *LIRS) handleHIRNonResidentBlock(block int) {
	LIRSObject.miss += 1
	LIRSObject.addToList(block)
	if _, ok := LIRSObject.orderedStack.Get(block); ok {

		LIRSObject.makeLIR(block)        // block x is in stack, move to LIR
		LIRSObject.removeFromList(block) // block x is in stack, remove from List Q
		LIRSObject.condition1(true)
	} else {
		//condition2:block is not in stack, move to end of list
		LIRSObject.makeHIR(block)
	}
	LIRSObject.addToStack(block)
}

func (LIRSObject *LIRS) addToStack(block int) {
	if _, ok := LIRSObject.orderedStack.Get(block); ok {
		LIRSObject.orderedStack.MoveLast(block)
		return
	}
	LIRSObject.orderedStack.Set(block, 1)
}

func (LIRSObject *LIRS) addToList(block int) {
	if LIRSObject.orderedList.Len() == LIRSObject.HIRSize {
		LIRSObject.orderedList.PopFirst()
	}
	LIRSObject.orderedList.Set(block, 1)
}

func (LIRSObject *LIRS) removeFromList(block int) {
	LIRSObject.orderedList.Delete(block)
}

func (LIRSObject *LIRS) makeLIR(block int) {
	LIRSObject.LIR[block] = 1
	LIRSObject.removeFromList(block)
	delete(LIRSObject.HIR, block)
}

func (LIRSObject *LIRS) makeHIR(block int) {
	LIRSObject.HIR[block] = 1
	delete(LIRSObject.LIR, block)
}

// condition1: move the LIR block in the bottom of stack S to the end of list Q with its status changed to HIR
func (LIRSObject *LIRS) condition1(removeLIR bool) (err error) {
	key, _, ok := LIRSObject.orderedStack.PopFirst()
	if !ok {
		return errors.New("orderedStack is empty")
	}
	if removeLIR {
		LIRSObject.makeHIR(key.(int))        //LIR block bottom becoming HIR
		LIRSObject.orderedList.Set(key, 1)   //inserted to list
		LIRSObject.orderedList.MoveLast(key) //inserted to end of the list
	}
	LIRSObject.stackPruning() //call pruning
	return nil
}

func (LIRSObject *LIRS) stackPruning() { //checking the next most bottom of the page
	iter := LIRSObject.orderedStack.Iter()
	for k, _, ok := iter.Next(); ok; k, _, ok = iter.Next() {
		if _, ok := LIRSObject.LIR[k]; ok { //check if  k block is LIR
			break //found LIR stop
		}
		LIRSObject.orderedStack.PopFirst() // if LIR not found, pop from the bottom of the stack
	}
}

func (LIRSObject *LIRS) PrintToFile(file *os.File, start time.Time) (err error) {
	duration := time.Since(start)
	hitRatio := 100 * float32(float32(LIRSObject.hit)/float32(LIRSObject.hit+LIRSObject.miss))
	result := fmt.Sprintf(`_______________________________________________________
LIRS
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
!LIRS|%v|%v|%v
`, LIRSObject.cacheSize, LIRSObject.hit, LIRSObject.miss, hitRatio, LIRSObject.orderedList.Len(), LIRSObject.orderedStack.Len(), LIRSObject.LIRSize, LIRSObject.HIRSize, LIRSObject.writeCount, duration.Seconds(), LIRSObject.cacheSize, LIRSObject.hit, LIRSObject.hit+LIRSObject.miss)
	_, err = file.WriteString(result)
	return err
}
