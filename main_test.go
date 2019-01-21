package main

import (
	"fmt"
	"testing"

	"github.com/t-drk/quiet_hn_1/hn"
)

func Test(t *testing.T) {
	numStories := 30
	obtained := make(chan hn.Result)
	go func() {
		obtained <- <-TopStoriesParallel(numStories)
	}()
	go func() {
		obtained <- <-TopStoriesSequential(numStories)
	}()
	val1 := <-obtained
	val2 := <-obtained
	if val1.Error != nil && val2.Error != nil {
		fmt.Println("Both of request for top item resulted in error.")
	} else if val1.Error != nil || val2.Error != nil {
		t.Errorf("One of request produced an error while other did not.\n")
	} else {
		stories1 := val1.Value.([]Item)
		stories2 := val2.Value.([]Item)
		if len(stories1) != len(stories2) {
			t.Errorf("Both the requests produced different amount of stories (%d, %d)\n", len(stories1), len(stories2))
		} else {
			var Error bool
			for i := range stories1 {
				if stories1[i].ID != stories2[i].ID {
					t.Errorf("Stories don't match at index [%d]\n Stories1: %#v\n Stories2: %#v\n",
						i, stories1, stories2)
					Error = true
				}
			}
			if !Error {
				fmt.Println("Both the stories matched")
			}
		}
	}
}
