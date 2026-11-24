package app

import (
	"fmt"

	"github.com/egoist/mygo/plugins/glass"
	"github.com/egoist/mygo/ui"

	"netease-music-mygo/internal/netease"
)

// userDetail and userPlaylists load a user and their playlists from
// NetEase, and followedArtists and subscribedPodcasts what the listener
// follows; tests replace them.
var (
	userDetail         = netease.UserDetail
	userPlaylists      = netease.UserPlaylists
	followedArtists    = netease.FollowedArtists
	subscribedPodcasts = netease.SubscribedPodcasts
)

// userData is what the app has of a user: who they are, and the playlists
// they made that anyone may see. Of the listener's own page, it has the
// artists they follow and the podcasts they subscribe to too, which
// NetEase shows no one else.
type userData struct {
	user      *netease.User
	playlists []netease.Playlist
	loading   int // how many of the two are on their way
	err       string

	artists       []netease.ArtistProfile
	podcasts      []netease.Podcast
	artistsShown  bool // whether artists has loaded once
	podcastsShown bool
	subsLoading   int
}

func (a *app) loadUser(id int64) {
	d := a.users[id]
	if d == nil {
		d = &userData{}
		a.users[id] = d
	}
	if a.profile != nil && a.profile.UserID == id {
		a.loadSubscriptions(d)
	}
	if d.user != nil || d.loading > 0 {
		return
	}
	d.loading, d.err = 2, ""
	go func() {
		u, err := userDetail(id)
		a.post(func() {
			d.loading--
			if err != nil {
				d.err = "用户加载失败"
				return
			}
			d.user = u
		})
	}()
	go func() {
		made, _, err := userPlaylists(id)
		a.post(func() {
			d.loading--
			if err == nil {
				d.playlists = publicPlaylists(made, id)
			}
		})
	}()
}

// loadSubscriptions loads the artists the listener follows and the
// podcasts they subscribe to, each time their page shows: they follow and
// subscribe on other pages, and these keep up. What they had shows while
// the lists are on their way.
func (a *app) loadSubscriptions(d *userData) {
	if d.subsLoading > 0 {
		return
	}
	d.subsLoading = 2
	// Snapshot request dependencies before starting asynchronous work.
	artists, podcasts := followedArtists, subscribedPodcasts
	go func() {
		ars, err := artists()
		a.post(func() {
			d.subsLoading--
			if err == nil {
				d.artists, d.artistsShown = ars, true
			}
		})
	}()
	go func() {
		pods, err := podcasts()
		a.post(func() {
			d.subsLoading--
			if err == nil {
				d.podcasts, d.podcastsShown = pods, true
			}
		})
	}()
}

// publicPlaylists are the playlists of lists a user made that anyone may
// see: their own, as made lists those they saved too, but not their
// private ones.
func publicPlaylists(lists []netease.Playlist, uid int64) []netease.Playlist {
	var out []netease.Playlist
	for _, pl := range lists {
		if pl.Creator.UserID == uid && pl.Privacy != 10 {
			out = append(out, pl)
		}
	}
	return out
}

func (a *app) userPage(c *ui.Context) {
	p := a.pal
	id := a.route.id
	d := a.users[id]
	if a.phone {
		a.userPhonePage(c, d)
		return
	}
	if d.user == nil {
		if d.loading == 0 {
			a.centered(c, d.err, func() { a.loadUser(id) })
			return
		}
		s := a.pageShimmer(c)
		a.scrollPage(c, func() {
			a.playlistHeaderSkeleton(c, s)
			a.playlistCardsSkeleton(c, s, a.columns(c, 168), 168, false)
		})
		return
	}
	u := d.user
	own := a.profile != nil && a.profile.UserID == id
	a.scrollPage(c, func() {
		a.detailHeader(c).Children(func() {
			ui.Box(c).Size(artistPhotoSize, artistPhotoSize).Radius(artistPhotoSize/2).Shadow(0, 18, 40, -10, p.cardShade).Children(func() {
				a.coverFill(c, u.AvatarURL, 600, artistPhotoSize/2)
			})
			ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(8).Children(func() {
				ui.Text(c, "用户").FontSize(12).FontWeight(600).TextColor(p.accent)
				ui.Text(c, u.Nickname).FontSize(30).FontWeight(800).LetterSpacing(-0.6).MaxLines(2).LineHeight(1.2)
				ui.Text(c, fmt.Sprintf("%s 关注 · %s 粉丝", count(u.Follows), count(u.Followeds))).FontSize(13).TextColor(p.muted)
				if u.Signature != "" {
					ui.Text(c, u.Signature).FontSize(12.5).TextColor(p.muted).MaxLines(2).LineHeight(1.45).Margin(2, 0, 0, 0)
				}
				// A user who is an artist has a page of their songs too.
				if u.ArtistID != 0 {
					ui.Row(c).Margin(10, 0, 0, 0).Children(func() {
						if pillButton(c.Key("artist"), iconNote, "歌手主页", p.hover.Over(p.bg), p.text).Clicked() {
							a.openPage(route{page: pageArtist, id: u.ArtistID})
						}
					})
				}
			})
		})
		a.section(c, "创建的歌单")
		switch {
		case d.loading > 0:
			a.playlistCardsSkeleton(c, a.pageShimmer(c), a.columns(c, 168), 168, false)
		case len(d.playlists) == 0:
			a.empty(c, "暂无公开歌单")
		default:
			ui.Grid(c).Columns(a.columns(c, 168)).GapX(20).GapY(28).Children(func() {
				for i := range d.playlists {
					a.playlistCard(c.Key(d.playlists[i].ID), &d.playlists[i])
				}
			})
		}
		if own && d.podcastsShown {
			a.section(c, "订阅的播客")
			if len(d.podcasts) == 0 {
				a.empty(c, "还没有订阅播客")
			}
			ui.Grid(c).Columns(a.columns(c, 340)).GapX(8).GapY(4).Margin(0, -12).Children(func() {
				for i := range d.podcasts {
					a.podcastRow(c.Key(d.podcasts[i].ID), &d.podcasts[i])
				}
			})
		}
		if own && d.artistsShown {
			a.section(c, "关注的歌手")
			if len(d.artists) == 0 {
				a.empty(c, "还没有关注歌手")
			}
			ui.Grid(c).Columns(a.columns(c, 132)).GapX(20).GapY(28).Children(func() {
				for i := range d.artists {
					a.artistCard(c.Key(d.artists[i].ID), &d.artists[i])
				}
			})
		}
	})
}

// userRow is a user in a list, opening their page.
func (a *app) userRow(c *ui.Context, u *netease.User) {
	p := a.pal
	row := ui.ButtonBase(c).Padding(10, 12).Gap(14).Radius(10).AlignItems(ui.Center).FocusRing(false).Label(u.Nickname)
	if row.Hovered() {
		row.Background(p.hover)
	}
	row.Children(func() {
		a.cover(c, u.AvatarURL, 120, 48, 24)
		ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(4).Children(func() {
			ui.Text(c, u.Nickname).FontSize(14).FontWeight(600).SingleLine()
			if u.Signature != "" {
				ui.Text(c, u.Signature).FontSize(12.5).TextColor(p.muted).SingleLine()
			}
		})
	})
	if row.Clicked() {
		a.open(route{page: pageUser, id: u.UserID})
	}
}

// userPhonePage is a user's page on the phone, as the mobile app's: the
// avatar blurred and darkened under the nav bar, the person over it, and
// a card of tabs over what they made and follow.
func (a *app) userPhonePage(c *ui.Context, d *userData) {
	id := a.route.id
	if d.user == nil {
		if d.loading == 0 {
			a.centered(c, d.err, func() { a.loadUser(id) })
			return
		}
		s := a.pageShimmer(c)
		a.scrollPage(c, func() {
			top, _ := a.insets(c)
			pad := a.pagePadding()
			ui.Box(c).Margin(-(top + pageGap), -pad, 0, -pad).Height(190).Background(a.pal.hover)
			ui.Row(c).Margin(-40, 0, 0, 0).PaddingX(4).Gap(14).AlignItems(ui.Center).Children(func() {
				s.bone(c, 38).Size(76, 76)
				ui.Column(c).Grow(1).Gap(8).Children(func() {
					s.text(c, 20, 1.2).Width(140)
					s.text(c, 13, 1.3).Width(90)
				})
			})
			ui.Column(c).Margin(16, -pad, 0, -pad).Radius(16, 16, 0, 0).Background(a.pal.bg).Padding(14, pad, 0, pad).Children(func() {
				for i := range 3 {
					if i > 0 {
						ui.Box(c).Height(1).Margin(0, 0, 0, 62).Background(a.pal.hairline)
					}
					ui.Row(c).Height(68).Gap(10).AlignItems(ui.Center).Children(func() {
						s.bone(c, 6).Size(52, 52)
						ui.Column(c).Grow(1).Gap(6).Children(func() {
							s.text(c, 16, 1.2).WidthPercent(55)
							s.text(c, 13, 1.3).WidthPercent(30)
						})
					})
				}
			})
		})
		return
	}
	u := d.user
	own := a.profile != nil && a.profile.UserID == id
	a.scrollPage(c, func() {
		p := a.pal
		top, _ := a.insets(c)
		pad := a.pagePadding()
		ui.Box(c.Key("user-head")).Margin(-(top + pageGap), -pad, 0, -pad).Clip().TextColor(white).Children(func() {
			a.coverFill(c, u.AvatarURL, 400, 0)
			ui.Box(c).Absolute().Top(0).Left(0).Right(0).Bottom(0).Material(glass.Blur{Radius: 30}).PassThrough()
			ui.Box(c).Absolute().Top(0).Left(0).Right(0).Bottom(0).PassThrough().Background(ui.RGBA(0, 0, 0, 0.35))
			ui.Row(c).ZIndex(1).Padding(top+pageGap+12, pad, 24, pad).Gap(14).AlignItems(ui.Center).Children(func() {
				ui.Box(c).Size(80, 80).Radius(40).Border(2, white.Alpha(0.8)).Shrink(0).Children(func() {
					a.coverFill(c, u.AvatarURL, 200, 40)
				})
				ui.Column(c).Grow(1).MinWidth(0).Gap(5).Children(func() {
					ui.Text(c, u.Nickname).FontSize(20).FontWeight(700).SingleLine()
					ui.Text(c, fmt.Sprintf("%s 关注 · %s 粉丝", count(u.Follows), count(u.Followeds))).FontSize(13).TextColor(white.Alpha(0.75)).SingleLine()
					if u.Signature != "" {
						ui.Text(c, u.Signature).FontSize(12).TextColor(white.Alpha(0.6)).MaxLines(2).LineHeight(1.3)
					}
				})
			})
		})
		// The tabbed card rides over the header's bottom, full-bleed.
		ui.Column(c).Margin(-16, -pad, 0, -pad).Radius(16, 16, 0, 0).Background(p.bg).Children(func() {
			labels := []string{"创建的歌单"}
			if own && d.podcastsShown && len(d.podcasts) > 0 {
				labels = append(labels, "订阅的播客")
			}
			if own && d.artistsShown && len(d.artists) > 0 {
				labels = append(labels, "关注的歌手")
			}
			if a.phoneUserTab >= len(labels) {
				a.phoneUserTab = 0
			}
			if len(labels) > 1 {
				ui.Box(c).Padding(8, pad, 0, pad).Children(func() {
					a.phoneTabs(c, &a.phoneUserTab, labels...)
				})
			}
			ui.Column(c).PaddingX(pad).Margin(4, 0, 0, 0).Children(func() {
				switch labels[a.phoneUserTab] {
				case "创建的歌单":
					switch {
					case d.loading > 0:
						s := a.pageShimmer(c)
						for range 3 {
							ui.Row(c).Height(68).Gap(10).AlignItems(ui.Center).Children(func() {
								s.bone(c, 6).Size(52, 52)
								ui.Column(c).Grow(1).Gap(6).Children(func() {
									s.text(c, 16, 1.2).WidthPercent(55)
									s.text(c, 13, 1.3).WidthPercent(30)
								})
							})
						}
					case len(d.playlists) == 0:
						a.empty(c, "暂无公开歌单")
					default:
						a.phonePlaylistRows(c, d.playlists, own)
					}
				case "订阅的播客":
					a.userPhonePodcasts(c, d.podcasts)
				case "关注的歌手":
					a.userPhoneArtists(c, d.artists)
				}
			})
		})
	})
}

// userPhonePodcasts lists the podcasts a user subscribes to on their
// phone page: a cover, the name and the count of episodes.
func (a *app) userPhonePodcasts(c *ui.Context, pods []netease.Podcast) {
	p := a.pal
	for i := range pods {
		pod := &pods[i]
		if i > 0 {
			ui.Box(c).Height(1).Margin(0, 0, 0, 62).Background(p.hairline)
		}
		b := ui.ButtonBase(c.Key(pod.ID)).Height(68).Gap(10).AlignItems(ui.Center).Justify(ui.Start).FocusRing(false).Label(pod.Name)
		if b.Pressed() || b.Hovered() {
			b.Background(p.hover)
		}
		b.Children(func() {
			a.cover(c, pod.PicURL, 120, 52, 6)
			ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
				ui.Text(c, pod.Name).FontSize(16).SingleLine()
				ui.Textf(c, "%d 期", pod.ProgramCount).FontSize(13).TextColor(p.muted).SingleLine()
			})
		})
		if b.Clicked() {
			a.open(route{page: pagePodcast, id: pod.ID})
		}
	}
}

// userPhoneArtists lists the artists a user follows on their phone page:
// a round photo and the name.
func (a *app) userPhoneArtists(c *ui.Context, artists []netease.ArtistProfile) {
	p := a.pal
	for i := range artists {
		ar := &artists[i]
		if i > 0 {
			ui.Box(c).Height(1).Margin(0, 0, 0, 58).Background(p.hairline)
		}
		b := ui.ButtonBase(c.Key(ar.ID)).Height(64).Gap(10).AlignItems(ui.Center).Justify(ui.Start).FocusRing(false).Label(ar.Name)
		if b.Pressed() || b.Hovered() {
			b.Background(p.hover)
		}
		b.Children(func() {
			photo := ar.Avatar
			if photo == "" {
				photo = ar.PicURL
			}
			ui.Box(c).Size(48, 48).Radius(24).Shrink(0).Children(func() {
				a.coverFill(c, photo, 120, 24)
			})
			ui.Text(c, ar.Name).FontSize(16).SingleLine().Grow(1).MinWidth(0)
		})
		if b.Clicked() {
			a.open(route{page: pageArtist, id: ar.ID})
		}
	}
}
