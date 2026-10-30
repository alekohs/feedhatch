package tui

// These are deliberately local fixtures, not fetched or persisted subscriptions.
func mockSubscriptions() []subscription {
	return []subscription{
		{
			name: "Arch Linux", url: "https://archlinux.org/feeds/news/", category: "Teknik",
			articles: []article{
				{title: "Nyheter från pakethyllan", date: "Idag · 09:42", unread: true, body: "Det här är en mockad artikel från Arch Linux. Inga nyheter har hämtats från nätet.\n\nEn rullande distribution handlar om små, löpande uppdateringar. Här finns plats för paketnyheter, ändringar och sådant som är bra att känna till innan nästa uppdatering.\n\nLäs i lugn och ro\n\nAnvänd j och k för att rulla texten. Med h eller Esc kommer du tillbaka till artikellistan. Artikeln markeras som läst när du öppnar den.\n\nEtt arbetsflöde i terminalen\n\nSamla dina källor i kategorier som passar dig. Teknik, utveckling, design eller något helt annat: kategorin är bara ett sätt att hitta tillbaka. Tryck c för att flytta ett flöde till en annan kategori.\n\nDet viktigaste behöver inte vara komplicerat. Några bra källor, en tydlig lista och en plats att läsa utan att tappa sammanhanget.\n\nOm den här förhandsvisningen\n\nAlla artiklar i den här versionen är exempeldata. Flöden du lägger till får en tom artikellista tills RSS-hämtningen kopplas in. Inga ändringar sparas när programmet avslutas."},
				{title: "Att tänka på före nästa uppdatering", date: "Igår · 16:10", unread: true, body: "Mockad artikel.\n\nLäs distributionsnyheterna innan en större uppdatering. Ibland behövs ett manuellt steg för att övergången ska gå smidigt.\n\nHär kommer hela artikeltexten att visas när hämtningen är ansluten."},
				{title: "Ett litet underhållsfönster", date: "27 okt · 12:00", body: "Mockad artikel.\n\nEn kort anteckning om underhåll, speglar och vardagen med ett system som alltid rör sig framåt."},
			},
		},
		{
			name: "Hacker News", url: "https://news.ycombinator.com/rss", category: "Teknik",
			articles: []article{
				{title: "Små verktyg som gör en sak bra", date: "Idag · 08:15", unread: true, body: "Mockad artikel.\n\nEtt verktyg behöver inte göra allt. En tydlig uppgift och bra samspel med andra verktyg räcker långt.\n\nTerminalen är en bra plats för program som prioriterar snabbhet, tangentbord och låg friktion."},
				{title: "Vad gör ett gränssnitt behagligt?", date: "Igår · 19:30", body: "Mockad artikel.\n\nFörutsägbar navigering, tydligt fokus och lagom mycket information. Det är ofta de små detaljerna som gör skillnad."},
			},
		},
		{
			name: "The Go Blog", url: "https://go.dev/blog/feed.atom", category: "Utveckling",
			articles: []article{
				{title: "Att bygga vidare med Go", date: "Idag · 07:30", unread: true, body: "Mockad artikel.\n\nBörja med ett litet fungerande program. Dela upp det när ansvaren blir tydliga, inte innan.\n\nStandardbiblioteket ger dig HTTP, tester och mycket annat. Låt paket ta hand om formatens egenheter och lägg din energi på appens beteende."},
				{title: "Tester utan externa beroenden", date: "28 okt · 10:00", body: "Mockad artikel.\n\nMed httptest kan du låta en lokal server spela rollen som din externa tjänst. Testerna blir snabba och oberoende av nätet."},
			},
		},
		{
			name: "Fältanteckningar", url: "https://example.com/design.xml", category: "Design",
			articles: []article{
				{title: "Hierarki utan dekoration", date: "Igår · 14:20", unread: true, body: "Mockad artikel.\n\nEn tunn linje kan skilja två sammanhang åt. En enda accent kan visa var du är. Resten får vara tyst.\n\nEtt bra läsverktyg låter innehållet dominera, inte ramen runt det."},
			},
		},
	}
}
