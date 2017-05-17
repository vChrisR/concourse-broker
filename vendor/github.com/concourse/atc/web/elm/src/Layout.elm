port module Layout exposing (Flags, Model, Msg, init, update, urlUpdate, view, subscriptions)

import Html exposing (Html)
import Html.Attributes as Attributes exposing (class, id)
import Html.App
import Login exposing (Msg(..))
import Navigation
import TopBar
import SideBar
import Routes
import SubPage
import Task exposing (Task)
import Favicon

port newUrl : (String -> msg) -> Sub msg

type alias Flags =
  { turbulenceImgSrc : String
  }

type alias NavIndex = Int

anyNavIndex : NavIndex
anyNavIndex = -1

type alias Model =
  { navIndex : NavIndex
  , subModel : SubPage.Model
  , topModel : TopBar.Model
  , sideModel : SideBar.Model
  , sidebarVisible : Bool
  , turbulenceImgSrc : String
  , route : Routes.ConcourseRoute
  }

type Msg
  = Noop
  | SubMsg NavIndex SubPage.Msg
  | TopMsg NavIndex TopBar.Msg
  | SideMsg NavIndex SideBar.Msg
  | NewUrl String

init : Flags -> Routes.ConcourseRoute -> (Model, Cmd (Msg))
init flags route =
  let
    (subModel, subCmd) =
      SubPage.init flags.turbulenceImgSrc route

    (topModel, topCmd) =
      TopBar.init route

    (sideModel, sideCmd) =
      SideBar.init

    navIndex =
      1
  in
    ( { navIndex = navIndex
      , subModel = subModel
      , topModel = topModel
      , sideModel = sideModel
      , sidebarVisible = False
      , turbulenceImgSrc = flags.turbulenceImgSrc
      , route = route
      }
    , Cmd.batch
        [ Cmd.map (SubMsg navIndex) subCmd
        , Cmd.map (TopMsg navIndex) topCmd
        , Cmd.map (SideMsg navIndex) sideCmd
        ]
    )

update : Msg -> Model -> (Model, Cmd (Msg))
update msg model =
  case msg of
    NewUrl url ->
      (model, Navigation.newUrl url)

    TopMsg _ TopBar.ToggleSidebar ->
      ( { model
        | sidebarVisible = not model.sidebarVisible
        }
      , Cmd.none
      )

    SubMsg navIndex (SubPage.LoginMsg (Login.LoginTokenReceived (Ok val))) ->
      let
        (subModel, subCmd) =
          SubPage.update model.turbulenceImgSrc (SubPage.LoginMsg (Login.LoginTokenReceived (Ok val))) model.subModel
      in
        ( { model
          | subModel = subModel
          }
        , Cmd.batch
            [ Cmd.map (TopMsg anyNavIndex) TopBar.fetchUser
            , Cmd.map (SideMsg anyNavIndex) SideBar.fetchPipelines
            , Cmd.map (SubMsg navIndex) subCmd
            ]
        )

    SubMsg navIndex (SubPage.PipelinesFetched (Ok pipelines)) ->
      let
        pipeline =
          List.head pipelines

        (subModel, subCmd) =
          SubPage.update
            model.turbulenceImgSrc
            (SubPage.DefaultPipelineFetched pipeline)
            model.subModel
      in
        case pipeline of
          Nothing ->
            ( { model
              | subModel = subModel
              }
            , Cmd.map (SubMsg navIndex) subCmd
            )
          Just p ->
            let
              (topModel, topCmd) =
                TopBar.update
                  (TopBar.FetchPipeline {teamName = p.teamName, pipelineName = p.name})
                  model.topModel
            in
              ( { model
                | subModel = subModel
                , topModel = topModel
                }
              , Cmd.batch
                  [ Cmd.map (SubMsg navIndex) subCmd
                  , Cmd.map (TopMsg navIndex) topCmd
                  ]
              )

    -- otherwise, pass down
    SubMsg navIndex m ->
      if (validNavIndex model.navIndex navIndex) then
        let
          (subModel, subCmd) = SubPage.update model.turbulenceImgSrc m model.subModel
        in
          ({ model | subModel = subModel }, Cmd.map (SubMsg navIndex) subCmd)
      else
        (model, Cmd.none)

    TopMsg navIndex m ->
      if (validNavIndex model.navIndex navIndex) then
        let
          (topModel, topCmd) = TopBar.update m model.topModel
        in
          ({ model | topModel = topModel }, Cmd.map (TopMsg navIndex) topCmd)
      else
        (model, Cmd.none)

    SideMsg navIndex m ->
      if (validNavIndex model.navIndex navIndex) then
        let
          (sideModel, sideCmd) = SideBar.update m model.sideModel
        in
          ({ model | sideModel = sideModel }, Cmd.map (SideMsg navIndex) sideCmd)
      else
        (model, Cmd.none)
    Noop ->
      (model, Cmd.none)

validNavIndex : NavIndex -> NavIndex -> Bool
validNavIndex modelNavIndex navIndex =
  if navIndex == anyNavIndex then
    True
  else
    navIndex == modelNavIndex

urlUpdate : Routes.ConcourseRoute -> Model -> (Model, Cmd (Msg))
urlUpdate route model =
  let
    navIndex =
      if route == model.route then
        model.navIndex
      else
        model.navIndex + 1

    (newSubmodel, cmd) =
      if route == model.route then
        (model.subModel, Cmd.none)
      else
        if routeMatchesModel route model then
          SubPage.urlUpdate route model.subModel
        else
          SubPage.init model.turbulenceImgSrc route

    (newTopModel, tCmd) =
      if route == model.route then
        (model.topModel, Cmd.none)
      else
        TopBar.urlUpdate route model.topModel
  in
    ( { model
      | navIndex = navIndex
      , subModel = newSubmodel
      , topModel = newTopModel
      , route = route
      }
    , Cmd.batch
        [ Cmd.map (SubMsg navIndex) cmd
        , Cmd.map (TopMsg navIndex) tCmd
        , resetFavicon
        ]
    )

resetFavicon : Cmd Msg
resetFavicon =
  Cmd.map (always Noop) << Task.perform Err Ok <|
    Favicon.set ("/public/images/favicon.png")

view : Model -> Html (Msg)
view model =
  let sidebarVisibileAppendage =
    case model.sidebarVisible of
      True ->
        " visible"
      False ->
        ""
  in
    Html.div [ class "content-frame" ]
      [ Html.div [ id "top-bar-app" ]
          [ Html.App.map (TopMsg model.navIndex) (TopBar.view model.topModel) ]
      , Html.div [ class "bottom" ]
          [ Html.div
              [ id "pipelines-nav-app"
              , class <| "sidebar test" ++ sidebarVisibileAppendage
              ]
              [ Html.App.map (SideMsg model.navIndex) (SideBar.view model.sideModel) ]
          , Html.div [ id "content" ]
              [ Html.div [ id "subpage" ]
                  [ Html.App.map (SubMsg model.navIndex) (SubPage.view model.subModel) ]
              ]
          ]
      ]

subscriptions : Model -> Sub Msg
subscriptions model =
  Sub.batch
    [ newUrl NewUrl
    , Sub.map (TopMsg model.navIndex) <| TopBar.subscriptions model.topModel
    , Sub.map (SideMsg model.navIndex) <| SideBar.subscriptions model.sideModel
    , Sub.map (SubMsg model.navIndex) <| SubPage.subscriptions model.subModel
    ]


routeMatchesModel : Routes.ConcourseRoute -> Model -> Bool
routeMatchesModel route model =
  case (route.logical, model.subModel) of
    (Routes.SelectTeam, SubPage.SelectTeamModel _) ->
      True
    (Routes.TeamLogin _, SubPage.LoginModel _) ->
      True
    (Routes.Pipeline _ _, SubPage.PipelineModel _) ->
      True
    (Routes.Resource _ _ _, SubPage.ResourceModel _) ->
      True
    (Routes.Build _ _ _ _, SubPage.BuildModel _) ->
      True
    (Routes.Job _ _ _, SubPage.JobModel _) ->
      True
    _ ->
      False
