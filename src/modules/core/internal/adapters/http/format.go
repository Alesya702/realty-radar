package httpadapter

import (
	"fmt"
	"html/template"
	"strconv"
	"strings"
	"time"

	"realtyradar/src/modules/core/internal/domain"
)

const emptyValue = "—"

func (h *Handler) funcs() template.FuncMap {
	return template.FuncMap{
		"money":        formatMoney,
		"area":         formatArea,
		"intp":         formatIntPtr,
		"strp":         formatStrPtr,
		"datetime":     h.formatDateTime,
		"datetimep":    h.formatDateTimePtr,
		"date":         formatDate,
		"datep":        formatDatePtr,
		"inputMoney":   inputMoney,
		"inputArea":    inputArea,
		"inputInt":     inputInt,
		"inputStr":     inputStr,
		"inputNow":     h.inputDateTime,
		"inputNowp":    func(t *time.Time) string { return h.inputDateTime(*t) },
		"inputDate":    func(t time.Time) string { return t.Format(dateLayout) },
		"lotType":      lotTypeLabel,
		"lotTypes":     func() []domain.LotType { return domain.LotTypes },
		"status":       statusLabel,
		"presence":     presenceLabel,
		"relationType": relationTypeLabel,
		"relTypes":     func() []domain.RelationType { return domain.RelationTypes },
		"sourceKind":   sourceKindLabel,
		"change":       changeLabel,
		"idp":          formatIDPtr,
	}
}

// formatMoney renders rubles with grouped thousands: "15 900 000 ₽", "235 905,04 ₽".
func formatMoney(m *domain.Money) string {
	if m == nil {
		return emptyValue
	}
	s := m.String()
	intPart, frac, _ := strings.Cut(s, ".")
	neg := strings.HasPrefix(intPart, "-")
	intPart = strings.TrimPrefix(intPart, "-")
	out := groupThousands(intPart)
	if frac != "00" {
		out += "," + frac
	}
	if neg {
		out = "−" + out
	}
	return out + " ₽"
}

func formatArea(a *domain.Area) string {
	if a == nil {
		return emptyValue
	}
	return strings.ReplaceAll(trimZeros(a.String()), ".", ",") + " м²"
}

func groupThousands(digits string) string {
	var b strings.Builder
	for i, r := range digits {
		if i > 0 && (len(digits)-i)%3 == 0 {
			b.WriteRune(' ')
		}
		b.WriteRune(r)
	}
	return b.String()
}

func trimZeros(s string) string {
	if !strings.Contains(s, ".") {
		return s
	}
	s = strings.TrimRight(s, "0")
	return strings.TrimSuffix(s, ".")
}

func formatIntPtr(v *int) string {
	if v == nil {
		return emptyValue
	}
	return strconv.Itoa(*v)
}

func formatStrPtr(v *string) string {
	if v == nil || *v == "" {
		return emptyValue
	}
	return *v
}

func formatIDPtr(id *domain.ID) string {
	if id == nil {
		return ""
	}
	return id.String()
}

func (h *Handler) formatDateTime(t time.Time) string {
	if t.IsZero() {
		return emptyValue
	}
	return t.In(h.loc).Format("02.01.2006 15:04")
}

func (h *Handler) formatDateTimePtr(t *time.Time) string {
	if t == nil {
		return emptyValue
	}
	return h.formatDateTime(*t)
}

func formatDate(t time.Time) string { return t.Format("02.01.2006") }

func formatDatePtr(t *time.Time) string {
	if t == nil {
		return emptyValue
	}
	return formatDate(*t)
}

func inputMoney(m *domain.Money) string {
	if m == nil {
		return ""
	}
	return trimZeros(m.String())
}

func inputArea(a *domain.Area) string {
	if a == nil {
		return ""
	}
	return trimZeros(a.String())
}

func inputInt(v *int) string {
	if v == nil {
		return ""
	}
	return strconv.Itoa(*v)
}

func inputStr(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

func (h *Handler) inputDateTime(t time.Time) string { return t.In(h.loc).Format(dateTimeLayout) }

func lotTypeLabel(t domain.LotType) string {
	switch t {
	case domain.LotTypeApartment:
		return "Квартира"
	case domain.LotTypeCommercial:
		return "Коммерческое помещение"
	case domain.LotTypeStorage:
		return "Кладовая"
	case domain.LotTypeParking:
		return "Машино-место"
	}
	return string(t)
}

func statusLabel(s domain.LotStatus) string {
	switch s {
	case domain.LotStatusAvailable:
		return "Свободен"
	case domain.LotStatusReserved:
		return "Забронирован"
	case domain.LotStatusSold:
		return "Продан"
	case domain.LotStatusUnknown:
		return "Неизвестно"
	}
	return string(s)
}

func presenceLabel(p domain.Presence) string {
	switch p {
	case domain.PresenceListed:
		return "В предложении"
	case domain.PresenceDelisted:
		return "Снят с публикации"
	}
	return string(p)
}

func relationTypeLabel(t domain.RelationType) string {
	switch t {
	case domain.RelationDirectCompetitor:
		return "Прямой конкурент"
	case domain.RelationIndirectCompetitor:
		return "Косвенный конкурент"
	case domain.RelationReference:
		return "Референсный проект"
	}
	return string(t)
}

func sourceKindLabel(k domain.DataSourceKind) string {
	switch k {
	case domain.DataSourceManual:
		return "Ручной ввод"
	case domain.DataSourceDocumentChannel:
		return "Канал документов"
	}
	return string(k)
}

func changeLabel(k domain.ChangeKind) string {
	switch k {
	case domain.ChangeCreated:
		return "создан"
	case domain.ChangeChanged:
		return "изменён"
	case domain.ChangeConfirmed:
		return "подтверждён без изменений"
	}
	return string(k)
}

var fieldLabels = map[string]string{
	"name":                  "Название",
	"city":                  "Город",
	"number":                "Номер",
	"lot_type":              "Тип лота",
	"floor":                 "Этаж",
	"rooms":                 "Количество комнат",
	"area_total":            "Общая площадь",
	"price":                 "Цена",
	"price_before_discount": "Цена до скидки",
	"status":                "Статус",
	"presence":              "Присутствие в предложении",
	"finish":                "Отделка",
	"observed_at":           "Дата наблюдения",
	"data_source":           "Источник данных",
	"data_source_id":        "Источник данных",
	"code":                  "Код",
	"kind":                  "Вид источника",
	"project":               "Проект",
	"related_project":       "Связанный проект",
	"related_project_id":    "Связанный проект",
	"relation_type":         "Тип отношения",
	"rationale":             "Обоснование",
	"valid_from":            "Дата начала",
	"valid_to":              "Дата окончания",
	"building":              "Корпус",
	"building_id":           "Корпус",
	"section":               "Секция",
	"source_url":            "URL источника",
	"source_key":            "Ключ источника",
	"note":                  "Примечание",
	"developer_id":          "Девелопер",
	"lot":                   "Лот",
	"at":                    "Дата и время",
}

var codeMessages = map[string]string{
	domain.CodeRequired:             "Заполните поле «%s».",
	domain.CodeTooLong:              "Поле «%s» слишком длинное.",
	domain.CodeInvalidValue:         "Недопустимое значение поля «%s».",
	domain.CodeInvalidNumber:        "Поле «%s»: введите число (не более двух знаков после запятой).",
	domain.CodeMustBePositive:       "Поле «%s» должно быть больше нуля.",
	domain.CodeMustNotBeNegative:    "Поле «%s» не может быть отрицательным.",
	domain.CodeOutOfRange:           "Поле «%s» вне допустимого диапазона.",
	domain.CodeDiscountWithoutPrice: "Цена до скидки указывается только вместе с ценой.",
	domain.CodeDiscountNotGreater:   "Цена до скидки должна быть больше цены. Если скидка явно не указана источником, оставьте поле пустым.",
	domain.CodeSectionWithoutBldg:   "Секция указывается только вместе с корпусом.",
	domain.CodePlacementMismatch:    "Корпус или секция не относятся к этому проекту (поле «%s»).",
	domain.CodeLotAlreadyExists:     "В проекте уже есть лот с таким типом, номером, корпусом и секцией. Откройте его карточку и добавьте наблюдение.",
	domain.CodeNameTaken:            "Объект с таким названием уже существует.",
	domain.CodeCodeTaken:            "Источник с таким кодом уже существует.",
	domain.CodeSelfRelation:         "Проект не может быть связан сам с собой.",
	domain.CodePeriodOrder:          "Дата окончания раньше даты начала.",
	domain.CodePeriodOverlap:        "Такая запись уже действует в этот период.",
	domain.CodeObservationInFuture:  "Дата наблюдения не может быть в будущем.",
	domain.CodeObservationOutOfOrdr: "Дата наблюдения раньше текущего состояния лота. Внесение истории задним числом в Core MVP не поддерживается.",
	domain.CodeHistoryConflict:      "Наблюдение с другими значениями не может быть раньше последнего подтверждения текущего состояния.",
	domain.CodeSourceInactive:       "Источник данных неактивен.",
	domain.CodeSourceKeyConflict:    "Ключ источника уже относится к другому лоту.",
	domain.CodeWrongProject:         "Объект относится к другому проекту (поле «%s»).",
}

func ruleMessage(re *domain.RuleError) string {
	label := fieldLabels[re.Field]
	if label == "" {
		label = re.Field
	}
	tmpl, ok := codeMessages[re.Code]
	if !ok {
		return "Некорректные данные: " + re.Error()
	}
	if strings.Contains(tmpl, "%s") {
		return fmt.Sprintf(tmpl, label)
	}
	return tmpl
}
