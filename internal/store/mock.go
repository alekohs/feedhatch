package store

import "time"

// SeedMock fills the store with sample feeds for tests and `make run-mock`.
func (s *Store) SeedMock() error {
	now := time.Now()
	day := func(offset, hour, minute int) time.Time {
		return time.Date(now.Year(), now.Month(), now.Day()+offset, hour, minute, 0, 0, time.Local)
	}

	mocks := []struct {
		feed     Feed
		articles []Article
	}{
		{
			feed: Feed{Name: "Arch Linux", URL: "https://archlinux.org/feeds/news/", Category: "Teknik"},
			articles: []Article{
				{GUID: "mock-1", Title: "Nyheter från pakethyllan", PublishedAt: day(0, 9, 42), Content: "Det här är en mockad artikel från Arch Linux. Inga nyheter har hämtats från nätet.\n\nEn rullande distribution handlar om små, löpande uppdateringar. Här finns plats för paketnyheter, ändringar och sådant som är bra att känna till innan nästa uppdatering.\n\nLäs i lugn och ro\n\nAnvänd j och k för att rulla texten. Med h eller Esc kommer du tillbaka till artikellistan. Artikeln markeras som läst när du öppnar den.\n\nEtt arbetsflöde i terminalen\n\nSamla dina källor i kategorier som passar dig. Teknik, utveckling, design eller något helt annat: kategorin är bara ett sätt att hitta tillbaka. Tryck c för att flytta ett flöde till en annan kategori.\n\nDet viktigaste behöver inte vara komplicerat. Några bra källor, en tydlig lista och en plats att läsa utan att tappa sammanhanget.\n\nOm den här förhandsvisningen\n\nAlla artiklar i den här versionen är exempeldata. Flöden du lägger till får en tom artikellista tills RSS-hämtningen kopplas in. Inga ändringar sparas när programmet avslutas."},
				{GUID: "mock-2", Title: "Att tänka på före nästa uppdatering", PublishedAt: day(-1, 16, 10), Content: "Mockad artikel.\n\nLäs distributionsnyheterna innan en större uppdatering. Ibland behövs ett manuellt steg för att övergången ska gå smidigt.\n\nHär kommer hela artikeltexten att visas när hämtningen är ansluten."},
				{GUID: "mock-3", Title: "Ett litet underhållsfönster", PublishedAt: day(-3, 12, 0), Read: true, Content: "Mockad artikel.\n\nEn kort anteckning om underhåll, speglar och vardagen med ett system som alltid rör sig framåt."},
			},
		},
		{
			feed: Feed{Name: "Hacker News", URL: "https://news.ycombinator.com/rss", Category: "Teknik"},
			articles: []Article{
				{GUID: "mock-4", Title: "Små verktyg som gör en sak bra", PublishedAt: day(0, 8, 15), Content: "Mockad artikel.\n\nEtt verktyg behöver inte göra allt. En tydlig uppgift och bra samspel med andra verktyg räcker långt.\n\nTerminalen är en bra plats för program som prioriterar snabbhet, tangentbord och låg friktion."},
				{GUID: "mock-5", Title: "Vad gör ett gränssnitt behagligt?", PublishedAt: day(-1, 19, 30), Read: true, Content: "Mockad artikel.\n\nFörutsägbar navigering, tydligt fokus och lagom mycket information. Det är ofta de små detaljerna som gör skillnad."},
			},
		},
		{
			feed: Feed{Name: "The Go Blog", URL: "https://go.dev/blog/feed.atom", Category: "Utveckling"},
			articles: []Article{
				{GUID: "mock-6", Title: "Att bygga vidare med Go", PublishedAt: day(0, 7, 30), Content: "Mockad artikel.\n\nBörja med ett litet fungerande program. Dela upp det när ansvaren blir tydliga, inte innan.\n\nStandardbiblioteket ger dig HTTP, tester och mycket annat. Låt paket ta hand om formatens egenheter och lägg din energi på appens beteende."},
				{GUID: "mock-7", Title: "Tester utan externa beroenden", PublishedAt: day(-2, 10, 0), Read: true, Content: "Mockad artikel.\n\nMed httptest kan du låta en lokal server spela rollen som din externa tjänst. Testerna blir snabba och oberoende av nätet."},
			},
		},
		{
			feed: Feed{Name: "Fältanteckningar", URL: "https://example.com/design.xml", Category: "Design"},
			articles: []Article{
				{GUID: "mock-8", Title: "Hierarki utan dekoration", PublishedAt: day(-1, 14, 20), Content: "Mockad artikel.\n\nEn tunn linje kan skilja två sammanhang åt. En enda accent kan visa var du är. Resten får vara tyst.\n\nEtt bra läsverktyg låter innehållet dominera, inte ramen runt det."},
			},
		},
	}

	for _, mock := range mocks {
		id, err := s.AddFeed(mock.feed)
		if err != nil {
			return err
		}
		if _, err := s.SaveArticles(id, mock.articles); err != nil {
			return err
		}
	}

	return nil
}
