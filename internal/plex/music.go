package plex

import (
 "bytes"
 "context"
 "errors"
 "fmt"
 "image/jpeg"
 "io"
 "net/http"
 "net/url"
 "strconv"
 "strings"
)

const MaxSearchResults = 36

// Playlists lists only music playlists. It requests one extra item to detect the next page.
func (c *Client) Playlists(ctx context.Context, offset int) ([]Item, bool, error) {
 if offset < 0 || offset > 100000 { return nil,false,errors.New("invalid playlist offset") }
 path:=fmt.Sprintf("/playlists?playlistType=audio&X-Plex-Container-Start=%d&X-Plex-Container-Size=%d",offset,pageSize+1)
 var result container
 if err:=c.Request(ctx,"GET",c.State.ServerURL,path,c.State.ServerToken,nil,&result);err!=nil{return nil,false,err}
 all:=result.MediaContainer.Metadata
 // Advance in raw Plex pages, not the filtered count, to avoid duplicate items.
 more:=len(all)>pageSize
 if more {all=all[:pageSize]}
 playlists:=make([]Item,0,len(all))
 for _,it:=range all {if it.Type=="playlist" && (it.PlaylistType==""||it.PlaylistType=="audio"){playlists=append(playlists,it)}}
 return playlists,more,nil
}

// PlaylistTracks keeps the server's ordering, including duplicate entries in
// playlists. Only metadata for up to maxItems tracks is held at one time.
func (c *Client) PlaylistTracks(ctx context.Context,id string,maxItems int)([]Item,error){
 if !ratingKey(id)||maxItems<1||maxItems>250{return nil,errors.New("invalid playlist or limit")}
 tracks:=make([]Item,0,min(maxItems,pageSize))
 for offset:=0;offset<maxItems;{
  page,more,err:=c.Items(ctx,"playlist",id,offset)
  if err!=nil{return nil,err}
  if len(page)==0 {break}
  for _,item:=range page{
   if item.Type!="track"||!ratingKey(item.RatingKey){continue}
   tracks=append(tracks,item)
   if len(tracks)>=maxItems{return tracks,nil}
  }
  offset+=len(page)
  if !more {break}
 }
 return tracks,nil
}

// Search uses the PMS hub search endpoint and keeps only music entities from
// the selected section. Video, photos, hubs and unsupported item kinds are dropped.
func (c *Client) Search(ctx context.Context,query string)([]Item,error){
 query=strings.TrimSpace(query)
 if query==""{return nil,nil}
 if len([]byte(query))>120{return nil,errors.New("search text too long")}
 params:=url.Values{}
 params.Set("query",query)
 params.Set("limit","15")
 if c.State.LibraryID!="" {params.Set("sectionId",c.State.LibraryID)}
 var result struct{
  MediaContainer struct{
   Hub []struct{Metadata []Item `json:"Metadata"`} `json:"Hub"`
   Metadata []Item `json:"Metadata"`
  } `json:"MediaContainer"`
 }
 if err:=c.Request(ctx,"GET",c.State.ServerURL,"/hubs/search?"+params.Encode(),c.State.ServerToken,nil,&result);err!=nil{return nil,err}
 out:=make([]Item,0,MaxSearchResults)
 seen:=make(map[string]bool,MaxSearchResults)
 add:=func(it Item){
  if it.Type!="artist"&&it.Type!="album"&&it.Type!="track"{return}
  if !ratingKey(it.RatingKey)||seen[it.Type+":"+it.RatingKey]||len(out)>=MaxSearchResults{return}
  seen[it.Type+":"+it.RatingKey]=true
  out=append(out,it)
 }
 for _,it:=range result.MediaContainer.Metadata{add(it)}
 for _,hub:=range result.MediaContainer.Hub{for _,it:=range hub.Metadata{add(it)}}
 return out,nil
}

// SetFavorite uses the Plex user's own five-star (rating=10) as a favorite.
// A lower existing rating is not silently preserved when the user explicitly
// selects "Favorite"; removing a favorite clears the rating (-1).
func (c *Client) SetFavorite(ctx context.Context,id string,favorite bool)error{
 if !ratingKey(id){return errors.New("invalid track id")}
 val:="-1"
 if favorite {val="10"}
 params:=url.Values{}
 params.Set("identifier","com.plexapp.plugins.library")
 params.Set("key",id)
 params.Set("rating",val)
 return c.Request(ctx,"PUT",c.State.ServerURL,"/:/rate?"+params.Encode(),c.State.ServerToken,nil,nil)
}

func (c *Client) FavoriteState(ctx context.Context,id string)(bool,error){
 item,err:=c.Track(ctx,id)
 if err!=nil{return false,err}
 return isFavorite(item),nil
}
func isFavorite(item Item)bool{return item.UserRating!=nil && *item.UserRating>=9.5}

// Plex only supplies a JPEG to the Q2 after the built-in photo transcoder has
// resized it. Never fetch arbitrary or external URLs in its name.
func validArtworkPath(s string)bool{
 if len(s)>400 || strings.Contains(s,".."){return false}
 for _,ch:=range s{if ch==0||ch==10||ch==13{return false}}
 
 if !(strings.HasPrefix(s,"/library/")||strings.HasPrefix(s,"/playlists/")){return false}
 u,err:=url.Parse(s)
 return err==nil && u.Host=="" && u.Scheme=="" && u.Fragment==""
}
func ratingKey(s string)bool{
 if s==""||len(s)>40{return false}
 for _,ch:=range s{if ch<'0'||ch>'9'{return false}}
 return true
}

// Thumbnail validates both JPEG dimensions and the downloaded byte count so
// a server cannot exhaust the Q2's RAM by returning an unscaled original.
func (c *Client) Thumbnail(ctx context.Context,thumb string,pixels int)([]byte,error){
 if !validArtworkPath(thumb){return nil,errors.New("untrusted artwork URL")}
 if pixels!=64 && pixels!=256{return nil,errors.New("unsupported artwork size")}
 params:=url.Values{}
 params.Set("width",strconv.Itoa(pixels));params.Set("height",strconv.Itoa(pixels))
 params.Set("minSize","1");params.Set("upscale","0");params.Set("format","jpeg")
 params.Set("url",thumb)
 root,err:=url.Parse(c.State.ServerURL);if err!=nil{return nil,err}
 if !allowedRoot(c.State.ServerURL){return nil,errors.New("untrusted server URL")}
 target:=root.ResolveReference(&url.URL{Path:"/photo/:/transcode",RawQuery:params.Encode()})
 req,err:=http.NewRequestWithContext(ctx,"GET",target.String(),nil);if err!=nil{return nil,err}
 c.headers(req,c.State.ServerToken)
 req.Header.Set("Accept","image/jpeg")
 resp,err:=c.HTTP.Do(req);if err!=nil{return nil,err}
 defer resp.Body.Close()
 if resp.StatusCode!=http.StatusOK{return nil,fmt.Errorf("art HTTP %d",resp.StatusCode)}
 limit:=int64(128<<10)
 if pixels==256{limit=512<<10}
 raw,err:=io.ReadAll(io.LimitReader(resp.Body,limit+1));if err!=nil{return nil,err}
 if int64(len(raw))>limit{return nil,errors.New("artwork is too large")}
 config,err:=jpeg.DecodeConfig(bytes.NewReader(raw))
 if err!=nil{return nil,fmt.Errorf("artwork not JPEG: %w",err)}
 if config.Width<1||config.Height<1||config.Width>pixels||config.Height>pixels{return nil,errors.New("artwork dimensions exceed limit")}
 return raw,nil
}
