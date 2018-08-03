package event

import (
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"time"

	"cloud.google.com/go/storage"
	"github.com/yuki-toida/knowme/config"
	"github.com/yuki-toida/knowme/domain/model"
	"github.com/yuki-toida/knowme/domain/repository"
)

// CouplesDay const
const CouplesDay = 4

// CouplesNight const
const CouplesNight = 9

const capacity = 3
const categoryDay = "day"
const categoryNight = "night"
const classDay = "text-white bg-danger rounded"
const classNight = "text-white bg-success rounded"

// UseCase type
type UseCase struct {
	EventRepository repository.Event
}

// NewUseCase func
func NewUseCase(u repository.Event) *UseCase {
	return &UseCase{
		EventRepository: u,
	}
}

// Get func
func (u *UseCase) Get(year, month, day int, category string, id string) *model.Event {
	return u.EventRepository.First(year, month, day, category, id)
}

// Gets func
func (u *UseCase) Gets() []*model.Event {
	events := u.EventRepository.FindAll()
	for _, v := range events {
		if v.Category == categoryDay {
			v.Classes = classDay
		} else {
			v.Classes = classNight
		}
	}
	return events
}

// GetUserEvent func
func (u *UseCase) GetUserEvent(user *model.User) *model.UserEvent {
	if user == nil {
		return nil
	}
	year := config.Now.Year()
	month := int(config.Now.Month())

	events := u.EventRepository.Find(&model.Event{Year: year, Month: month, ID: user.ID})
	if len(events) <= 0 {
		return nil
	}
	event := events[0]
	titles := []string{}
	for _, v := range u.EventRepository.Find(&model.Event{StartDate: event.StartDate, Category: event.Category}) {
		if v.StartDate == event.StartDate && v.Category == event.Category {
			titles = append(titles, v.Title)
		}
	}

	return &model.UserEvent{
		Date:     event.StartDate,
		Category: event.Category,
		Titles:   titles,
	}
}

// GetUserEvents func
func (u *UseCase) GetUserEvents(user *model.User) []*model.UserEvent {
	results := []*model.UserEvent{}
	if user == nil {
		return results
	}
	events := u.EventRepository.Find(&model.Event{ID: user.ID})
	if len(events) <= 0 {
		return results
	}
	for _, v := range events {
		date := time.Date(v.Year, time.Month(v.Month), v.Day, 0, 0, 0, 0, config.JST)
		results = append(results, &model.UserEvent{
			Date:     date,
			Category: v.Category,
			Titles:   []string{},
		})
	}
	return results
}

// GetRestCouples func
func (u *UseCase) GetRestCouples() (int, int) {
	year := config.Now.Year()
	month := int(config.Now.Month())
	dayRestCouples := CouplesDay - countCouples(u.EventRepository, year, month, categoryDay)
	nightRestCouples := CouplesNight - countCouples(u.EventRepository, year, month, categoryNight)
	return dayRestCouples, nightRestCouples
}

func countCouples(r repository.Event, year, month int, category string) int {
	events := r.Find(&model.Event{Year: year, Month: month, Category: category})
	eventMap := map[int][]string{}
	for _, v := range events {
		if val, ok := eventMap[v.Day]; ok {
			eventMap[v.Day] = append(val, v.ID)
		} else {
			eventMap[v.Day] = []string{v.ID}
		}
	}

	result := 0
	for _, v := range eventMap {
		if capacity <= len(v) {
			result++
		}
	}
	return result
}

// GetPictures func
func (u *UseCase) GetPictures() []string {
	year := config.Now.Year()
	month := int(config.Now.Month())

	events := u.EventRepository.Find(&model.Event{Year: year, Month: month})
	rootPath := config.Config.Server.StorageURL + "/" + config.Config.Server.Bucket
	results := []string{}
	for _, v := range events {
		if v.Ext != "" {
			url := rootPath + fmt.Sprintf("/%d/%d/%d/", year, month, v.Day) + v.Category + v.Ext
			results = append(results, url)
		}
	}
	return results
}

// GetAllPictures func
func (u *UseCase) GetAllPictures() map[time.Time][]string {
	events := u.EventRepository.FindAll()
	rootPath := config.Config.Server.StorageURL + "/" + config.Config.Server.Bucket
	dict := map[time.Time][]string{}
	for _, v := range events {
		if v.Ext != "" {
			url := rootPath + fmt.Sprintf("/%d/%d/%d/", v.Year, v.Month, v.Day) + v.Category + v.Ext
			date := time.Date(v.Year, time.Month(v.Month), 1, 0, 0, 0, 0, config.JST)
			if val, ok := dict[date]; ok {
				dict[date] = append(val, url)
			} else {
				dict[date] = []string{url}
			}
		}
	}
	return dict
}

// CreateEvent func
func (u *UseCase) CreateEvent(user *model.User, category string, date time.Time) (*model.Event, error) {
	today := time.Date(config.Now.Year(), config.Now.Month(), config.Now.Day(), 0, 0, 0, 0, config.JST)
	if date.Before(today) {
		return nil, errors.New("過去の登録は出来ません")
	}
	year := date.Year()
	month := int(date.Month())
	if today.Year() < year || int(today.Month()) < month {
		return nil, errors.New("未来の登録は出来ません")
	}
	if 0 < len(u.EventRepository.Find(&model.Event{Year: year, Month: month, ID: user.ID})) {
		return nil, errors.New("今月は既に登録済みです")
	}
	dateCount := len(u.EventRepository.Find(&model.Event{Year: year, Month: month, StartDate: date, Category: category}))
	if capacity <= dateCount {
		return nil, errors.New("すでに満席です")
	}

	countCouples := countCouples(u.EventRepository, year, month, category)
	var classes string
	switch category {
	case categoryDay:
		if CouplesDay <= countCouples {
			return nil, errors.New("昼Knowmeはすでに満席です")
		}
		classes = classDay
	case categoryNight:
		if CouplesNight <= countCouples {
			return nil, errors.New(category + "夜Knowmeはすでに満席です")
		}
		classes = classNight
	}

	if duplicateIds := u.duplicateIds(year, month, date, category, user.ID); 0 < len(duplicateIds) {
		message := strings.Join(duplicateIds, " ")
		return nil, errors.New(message + "と既に参加済みです")
	}

	event := &model.Event{
		Year:      date.Year(),
		Month:     int(date.Month()),
		Day:       date.Day(),
		Category:  category,
		ID:        user.ID,
		EventID:   date.Format("2006-01-02") + ":" + user.ID,
		Title:     user.Name,
		StartDate: date,
		EndDate:   date,
		Classes:   classes,
	}
	u.EventRepository.Create(event)
	return event, nil
}

func (u *UseCase) duplicateIds(year, month int, date time.Time, category, id string) []string {
	years := u.EventRepository.Find(&model.Event{Year: year})
	events := []*model.Event{}
	for _, v := range years {
		if v.ID == id {
			events = append(events, v)
		}
	}
	duplicateIds := []string{}
	for _, x := range years {
		for _, y := range events {
			if x.StartDate == y.StartDate && x.Category == y.Category {
				if x.ID != y.ID {
					duplicateIds = append(duplicateIds, x.ID)
				}
			}
		}
	}
	results := []string{}
	for _, x := range years {
		if x.StartDate == date && x.Category == category {
			for _, y := range duplicateIds {
				if x.ID == y {
					results = append(results, x.ID)
				}
			}
		}
	}
	return results
}

// Delete func
func (u *UseCase) Delete(id, category string, date time.Time) (*model.Event, error) {
	event := u.Get(date.Year(), int(date.Month()), date.Day(), category, id)
	if event == nil {
		return nil, errors.New("参加していません")
	}
	u.EventRepository.Delete(event)
	return event, nil
}

// Upload func
func (u *UseCase) Upload(year, month, day int, category, id, fileName string) error {
	event := u.Get(year, month, day, category, id)
	if event == nil {
		return errors.New("パラメータが不正です")
	}
	ctx := context.Background()
	client, err := storage.NewClient(ctx)
	if err != nil {
		return err
	}
	defer client.Close()

	data, err := ioutil.ReadFile(fileName)
	if err != nil {
		return err
	}

	ext := filepath.Ext(fileName)
	path := fmt.Sprintf("%d/%d/%d/", year, month, day) + category + ext
	w := client.Bucket(config.Config.Server.Bucket).Object(path).NewWriter(ctx)
	defer w.Close()

	if _, err := w.Write(data); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	if err := os.Remove(fileName); err != nil {
		panic(err)
	}
	event.Ext = ext
	u.EventRepository.Update(event)

	for _, v := range u.EventRepository.Find(&model.Event{Year: year, Month: month, Day: day, Category: category}) {
		if v.ID != id {
			v.Ext = ""
			u.EventRepository.Update(v)
		}
	}

	return nil
}
