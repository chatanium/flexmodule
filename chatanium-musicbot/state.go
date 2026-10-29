package main

import (
	"errors"
	"sync"
	"time"

	Provider "github.com/thirdscam/chatanium-flexmodule/chatanium-musicbot/provider"
)

type ChannelID string

var states map[ChannelID]*State = make(map[ChannelID]*State)

var (
	errEmptyQueue            = errors.New("queue is empty")
	errIndexOutOfRange       = errors.New("index is out of range")
	errIndexCannotBeNegative = errors.New("index cannot be negative")
	errSignalTimeout         = errors.New("signal timeout")
)

func GetState(channelID ChannelID) *State {
	if _, exists := states[channelID]; exists {
		return states[channelID]
	}

	states[channelID] = &State{
		queue:     []Provider.Music{},
		isPlaying: false,
		loop:      false,
		pause:     make(chan bool),
		skip:      make(chan bool),
	}

	return states[channelID]
}

type State struct {
	sync.RWMutex
	queue     []Provider.Music
	isPlaying bool
	loop      bool
	pause     chan bool
	skip      chan bool
}

func (s *State) GetQueue() []Provider.Music {
	s.RLock()
	defer s.RUnlock()
	return s.queue
}

func (s *State) IsQueueEmpty() bool {
	s.RLock()
	defer s.RUnlock()
	return len(s.queue) == 0
}

func (s *State) Enqueue(music ...Provider.Music) error {
	s.Lock()
	defer s.Unlock()
	s.queue = append(s.queue, music...)
	return nil
}

func (s *State) Dequeue() error {
	s.Lock()
	defer s.Unlock()

	if len(s.queue) == 0 {
		return errEmptyQueue
	}

	s.queue = s.queue[1:]
	return nil
}

func (s *State) Remove(index int) (Provider.Music, error) {
	s.Lock()
	defer s.Unlock()

	if index < 0 {
		return Provider.Music{}, errIndexCannotBeNegative
	}

	if len(s.queue) == 0 {
		return Provider.Music{}, errEmptyQueue
	}

	if index >= len(s.queue) {
		return Provider.Music{}, errIndexOutOfRange
	}

	target := s.queue[index]
	s.queue = append(s.queue[:index], s.queue[index+1:]...)

	return target, nil
}

func (s *State) Insert(index int, music ...Provider.Music) error {
	s.Lock()
	defer s.Unlock()

	if index < 0 {
		return errIndexCannotBeNegative
	}

	if index > len(s.queue) {
		return errIndexOutOfRange
	}

	s.queue = append(s.queue[:index], append(music, s.queue[index:]...)...)
	return nil
}

func (s *State) ToggleLoop() bool {
	s.Lock()
	defer s.Unlock()

	s.loop = !s.loop
	return s.loop
}

func (s *State) IsLoopMode() bool {
	return s.loop
}

func (s *State) GetFront() Provider.Music {
	if len(s.queue) == 0 {
		return Provider.Music{}
	}
	return s.queue[0]
}

func (s *State) SetIsPlaying(isRunning bool) {
	s.Lock()
	defer s.Unlock()
	s.isPlaying = isRunning
}

func (s *State) IsPlaying() bool {
	s.RLock()
	defer s.RUnlock()
	return s.isPlaying
}

func (s *State) Pop() Provider.Music {
	if len(s.queue) == 0 {
		return Provider.Music{}
	}

	target := s.queue[0]
	s.queue = s.queue[1:]
	return target
}

func (s *State) GetSignals() (chan bool, chan bool) {
	return s.pause, s.skip
}

func (s *State) IsExistMusic(music Provider.Music) bool {
	s.Lock()
	defer s.Unlock()

	for _, m := range s.queue {
		if m.Id == music.Id {
			return true
		}
	}
	return false
}

func (s *State) Pause() error {
	s.Lock()
	defer s.Unlock()

	if len(s.queue) == 0 {
		return errEmptyQueue
	}

	done := make(chan bool)
	go func() {
		s.pause <- true
		done <- true
	}()

	select {
	case <-time.After(3 * time.Second):
		return errSignalTimeout
	case <-done:
		return nil
	}
}

func (s *State) Skip() error {
	s.Lock()
	defer s.Unlock()

	if len(s.queue) == 0 {
		return errEmptyQueue
	}

	done := make(chan bool)
	go func() {
		s.skip <- true
		done <- true
	}()

	select {
	case <-time.After(3 * time.Second):
		return errSignalTimeout
	case <-done:
		return nil
	}
}
