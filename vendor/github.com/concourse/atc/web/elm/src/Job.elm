port module Job exposing (Flags, Model, changeToJob, subscriptions, init, update, view, Msg(..))

import Dict exposing (Dict)
import Html exposing (Html)
import Html.Attributes exposing (class, href, id, disabled, attribute)
import Html.Events exposing (onClick)
import Http
import Task
import Time exposing (Time)

import Concourse
import Concourse.Build
import Concourse.Job
import Concourse.BuildStatus
import Concourse.Pagination exposing (Pagination, Paginated, Page)
import Concourse.BuildResources exposing (fetch)
import BuildDuration
import DictView
import Navigation
import StrictEvents exposing (onLeftClick)
import LoginRedirect

type alias Ports =
  { title : String -> Cmd Msg
  }

type alias Model =
  { ports : Ports
  , jobIdentifier : Concourse.JobIdentifier
  , job : (Maybe Concourse.Job)
  , pausedChanging : Bool
  , buildsWithResources : Paginated BuildWithResources
  , currentPage : Maybe Page
  , now : Time
  }

type Msg
  = Noop
  | BuildTriggered (Result Http.Error Concourse.Build)
  | TriggerBuild
  | JobBuildsFetched (Result Http.Error (Paginated Concourse.Build))
  | JobFetched (Result Http.Error Concourse.Job)
  | BuildResourcesFetched Int (Result Http.Error Concourse.BuildResources)
  | ClockTick Time
  | TogglePaused
  | PausedToggled (Result Http.Error ())
  | NavTo String
  | SubscriptionTick Time

type alias BuildWithResources =
  { build : Concourse.Build
  , resources : Maybe Concourse.BuildResources
  }

jobBuildsPerPage : Int
jobBuildsPerPage = 100


type alias Flags =
  { jobName : String
  , teamName : String
  , pipelineName : String
  , paging : Maybe Page
  }

init : Ports -> Flags -> (Model, Cmd Msg)
init ports flags =
  let
    (model, cmd) =
      changeToJob flags
        { jobIdentifier =
            { jobName = flags.jobName
            , teamName = flags.teamName
            , pipelineName = flags.pipelineName
            }
        , job = Nothing
        , pausedChanging = False
        , buildsWithResources =
          { content = []
            , pagination =
              { previousPage = Nothing
              , nextPage = Nothing
              }
          }
        , now = 0
        , currentPage = flags.paging
        , ports = ports
        }
  in
    ( model
    , Cmd.batch
        [ fetchJob model.jobIdentifier
        , cmd
        , getCurrentTime
        ]
    )


changeToJob : Flags -> Model -> (Model, Cmd Msg)
changeToJob flags model =
  let
    model =
      { model
      | currentPage = flags.paging
      , buildsWithResources =
        { content = []
        , pagination =
            { previousPage = Nothing
            , nextPage = Nothing
            }
        }
      }
  in
    ( model
    , fetchJobBuilds model.jobIdentifier model.currentPage
    )

update : Msg -> Model -> (Model, Cmd Msg)
update action model =
  case action of
    Noop ->
      (model, Cmd.none)
    TriggerBuild ->
      (model, triggerBuild model.jobIdentifier)
    BuildTriggered (Ok build) ->
      ( model
      , case build.job of
          Nothing ->
            Cmd.none
          Just job ->
            Navigation.newUrl <|
              "/teams/" ++ job.teamName ++
              "/pipelines/" ++ job.pipelineName ++
              "/jobs/" ++ job.jobName ++
              "/builds/" ++ build.name
      )
    BuildTriggered (Err (Http.BadResponse 401 _)) ->
      (model, LoginRedirect.requestLoginRedirect "")
    BuildTriggered (Err err) ->
      flip always (Debug.log("failed to trigger build") (err) ) <|
        (model, Cmd.none)
    JobBuildsFetched (Ok builds) ->
      handleJobBuildsFetched builds model
    JobBuildsFetched (Err err) ->
      flip always (Debug.log("failed to fetch builds") (err) ) <|
        (model, Cmd.none)
    JobFetched (Ok job) ->
      ( { model | job = Just job }
      , model.ports.title <| job.name ++ " - "
      )
    JobFetched (Err (Http.BadResponse 401 _)) ->
      (model, LoginRedirect.requestLoginRedirect "")
    JobFetched (Err err) ->
      flip always (Debug.log("failed to fetch job info") (err) ) <|
        (model, Cmd.none)
    BuildResourcesFetched id (Ok buildResources) ->
      case model.buildsWithResources.content of
        [] ->
          (model, Cmd.none)
        anyList ->
          let
            transformer =
              \bwr ->
                let
                  bwrb =
                    bwr.build
                in
                  if bwr.build.id == id then
                    { bwr
                    | resources = Just buildResources
                    }
                  else
                    bwr
            bwrs =
              model.buildsWithResources
          in
            ( { model
              | buildsWithResources =
                  { bwrs
                  | content = List.map transformer anyList
                  }
              }
            , Cmd.none
            )
    BuildResourcesFetched _ (Err err) ->
      (model, Cmd.none)
    ClockTick now ->
      ({ model | now = now }, Cmd.none)
    TogglePaused ->
      case model.job of
        Nothing -> (model, Cmd.none)
        Just j ->
          ( { model
            | pausedChanging = True
            , job = Just { j | paused = not j.paused }
            }
          , if j.paused
            then unpauseJob model.jobIdentifier
            else pauseJob model.jobIdentifier
          )
    PausedToggled (Ok ()) ->
      ( { model | pausedChanging = False} , Cmd.none)
    PausedToggled (Err (Http.BadResponse 401 _)) ->
      (model, LoginRedirect.requestLoginRedirect "")
    PausedToggled (Err err) ->
      flip always (Debug.log("failed to pause/unpause job") (err) ) <|
        (model, Cmd.none)
    NavTo url ->
      (model, Navigation.newUrl url)
    SubscriptionTick time ->
      ( model
      , Cmd.batch
          [ fetchJobBuilds model.jobIdentifier model.currentPage
          , fetchJob model.jobIdentifier
          ]
      )

permalink : List Concourse.Build -> Page
permalink builds =
  case List.head builds of
    Nothing ->
      { direction = Concourse.Pagination.Since 0
      , limit = jobBuildsPerPage
      }
    Just build ->
      { direction = Concourse.Pagination.Since (build.id + 1)
      , limit = List.length builds
      }

paginatedMap : (a -> b) -> Paginated a -> Paginated b
paginatedMap promoter pagA =
  { content =
      List.map promoter pagA.content
  , pagination = pagA.pagination
  }

setResourcesToOld : Maybe BuildWithResources -> BuildWithResources -> BuildWithResources
setResourcesToOld existingBuildWithResource newBwr =
  case existingBuildWithResource of
    Nothing ->
      newBwr
    Just buildWithResources ->
      { newBwr
      | resources = buildWithResources.resources
      }

existingBuild : Concourse.Build -> BuildWithResources -> Bool
existingBuild build buildWithResources =
  build == buildWithResources.build

promoteBuild : Model -> Concourse.Build -> BuildWithResources
promoteBuild model build =
  let
    newBwr =
      { build = build
      , resources = Nothing
      }

    existingBuildWithResource =
      List.head
        (List.filter (existingBuild build) model.buildsWithResources.content)

  in
    setResourcesToOld existingBuildWithResource newBwr

setExistingResources : Paginated Concourse.Build -> Model -> Paginated BuildWithResources
setExistingResources paginatedBuilds model =
  paginatedMap (promoteBuild model) paginatedBuilds

updateResourcesIfNeeded : BuildWithResources -> Maybe (Cmd Msg)
updateResourcesIfNeeded bwr =
  case (bwr.resources, isRunning bwr.build) of
    (Just resources, False) ->
      Nothing
    _ ->
      Just <| fetchBuildResources bwr.build.id


handleJobBuildsFetched : Paginated Concourse.Build -> Model -> (Model, Cmd Msg)
handleJobBuildsFetched paginatedBuilds model =
  let
    newPage =
      permalink paginatedBuilds.content

    newBWRs =
      setExistingResources paginatedBuilds model
  in
    ( { model
      | buildsWithResources = newBWRs
      , currentPage = Just newPage
      }

      , Cmd.batch <| List.filterMap updateResourcesIfNeeded newBWRs.content
    )

isRunning : Concourse.Build -> Bool
isRunning build =
  Concourse.BuildStatus.isRunning build.status

view : Model -> Html Msg
view model =
  Html.div [class "with-fixed-header"]
  [
    case model.job of
      Nothing ->
        loadSpinner
      Just job ->
        Html.div [class "fixed-header"]
          [ Html.div [ class ("build-header " ++ headerBuildStatusClass job.finishedBuild)] -- TODO really?
              [ Html.button
                  ( List.append
                    [ id "job-state"
                    , attribute "aria-label" "Toggle Job Paused State"
                    , class <|
                        "btn-pause btn-large fl " ++ (getPausedState job model.pausedChanging)
                    ]
                    (if not model.pausedChanging then [onClick TogglePaused] else [])
                  )
                  [ Html.i
                    [ class <|
                        "fa fa-fw fa-play " ++ (getPlayPauseLoadIcon job model.pausedChanging)
                    ] []
                  ]
              , Html.form
                  [ class "trigger-build"
                  , onLeftClick TriggerBuild
                  ]
                  [ Html.button
                    [ class "build-action fr"
                    , disabled job.disableManualTrigger
                    , attribute "aria-label" "Trigger Build"
                    ]
                    [ Html.i [ class "fa fa-plus-circle" ] []
                    ]
                  ]
              , Html.h1 [] [ Html.text(model.jobIdentifier.jobName) ]
              ]
          , Html.div [ class "pagination-header" ]
              [ viewPaginationBar model
              , Html.h1 [] [ Html.text("builds") ]
              ]
          ],
    case model.buildsWithResources.content of
      [] ->
        loadSpinner

      anyList ->
        Html.div [class "scrollable-body job-body"]
          [ Html.ul [ class "jobs-builds-list builds-list" ]
            <| List.map (viewBuildWithResources model) anyList
          ]
  ]

getPlayPauseLoadIcon : Concourse.Job -> Bool -> String
getPlayPauseLoadIcon job pausedChanging =
  if pausedChanging then
    "fa-circle-o-notch fa-spin"
  else if job.paused then
    ""
  else
    "fa-pause"

getPausedState : Concourse.Job -> Bool -> String
getPausedState job pausedChanging =
  if pausedChanging then
    "loading"
  else if job.paused then
    "enabled"
  else
    "disabled"

loadSpinner : Html Msg
loadSpinner =
  Html.div [class "build-step"]
    [ Html.div [class "header"]
      [ Html.i [class "left fa fa-fw fa-spin fa-circle-o-notch"] []
      , Html.h3 [] [Html.text "Loading..."]
      ]
    ]

headerBuildStatusClass : (Maybe Concourse.Build) -> String
headerBuildStatusClass finishedBuild =
  case finishedBuild of
    Nothing -> ""
    Just build -> Concourse.BuildStatus.show build.status

viewPaginationBar : Model -> Html Msg
viewPaginationBar model =
  Html.div [ class "pagination fr"]
    [ case model.buildsWithResources.pagination.previousPage of
        Nothing ->
          Html.div [ class "btn-page-link disabled"]
          [ Html.span [class "arrow"]
            [ Html.i [ class "fa fa-arrow-left"] []
            ]
          ]
        Just page ->
          let
            jobUrl =
              "/teams/" ++ model.jobIdentifier.teamName ++ "/pipelines/" ++ model.jobIdentifier.pipelineName ++ "/jobs/"
              ++ model.jobIdentifier.jobName ++ "?" ++ paginationParam page
          in
            Html.div [ class "btn-page-link"]
            [ Html.a
              [ class "arrow"
              , StrictEvents.onLeftClick <| NavTo jobUrl
              , href jobUrl
              , attribute "aria-label" "Previous Page"
              ]
              [ Html.i [ class "fa fa-arrow-left"] []
              ]
            ]
    , case model.buildsWithResources.pagination.nextPage of
        Nothing ->
          Html.div [ class "btn-page-link disabled"]
          [ Html.span [class "arrow"]
            [ Html.i [ class "fa fa-arrow-right"] []
            ]
          ]
        Just page ->
          let
            jobUrl =
              "/teams/" ++ model.jobIdentifier.teamName ++ "/pipelines/" ++ model.jobIdentifier.pipelineName ++ "/jobs/"
                ++ model.jobIdentifier.jobName ++ "?" ++ paginationParam page
          in
            Html.div [ class "btn-page-link"]
            [ Html.a
              [ class "arrow"
              , StrictEvents.onLeftClick <| NavTo jobUrl
              , href jobUrl
              , attribute "aria-label" "Next Page"
              ]
              [ Html.i [ class "fa fa-arrow-right"] []
              ]
            ]
    ]

viewBuildWithResources : Model -> BuildWithResources -> Html Msg
viewBuildWithResources model bwr =
  Html.li [class "js-build"]
    <| let
      buildResourcesView = viewBuildResources model bwr
    in
      [ viewBuildHeader model bwr.build
      , Html.div [class "pam clearfix"]
        <| (BuildDuration.view bwr.build.duration model.now) :: buildResourcesView
      ]

viewBuildHeader : Model -> Concourse.Build -> Html Msg
viewBuildHeader model b =
  Html.a
  [class <| Concourse.BuildStatus.show b.status
  , StrictEvents.onLeftClick <| NavTo <| Concourse.Build.url b
  , href <| Concourse.Build.url b
  ]
  [ Html.text ("#" ++ b.name)
  ]

viewBuildResources : Model -> BuildWithResources -> List (Html Msg)
viewBuildResources model buildWithResources =
  let
    inputsTable =
      case buildWithResources.resources of
        Nothing ->
          loadSpinner
        Just resources ->
          Html.table [class "build-resources"] <|
            List.map (viewBuildInputs model) resources.inputs
    outputsTable =
      case buildWithResources.resources of
        Nothing ->
          loadSpinner
        Just resources ->
          Html.table [class "build-resources"] <|
            List.map (viewBuildOutputs model) resources.outputs
  in
    [ Html.div [class "inputs mrl"]
      [ Html.div [class "resource-title pbs"]
        [ Html.i [class "fa fa-fw fa-arrow-down prs"] []
        , Html.text("inputs")
        ]
      , inputsTable
      ]
    , Html.div [class "outputs mrl"]
      [ Html.div [class "resource-title pbs"]
        [ Html.i [class "fa fa-fw fa-arrow-up prs"] []
        , Html.text("outputs")
        ]
      , outputsTable
      ]
    ]

viewBuildInputs : Model -> Concourse.BuildResourcesInput -> Html Msg
viewBuildInputs model bi =
  Html.tr [class "mbs pas resource fl clearfix"]
  [ Html.td [class "resource-name mrm"]
    [ Html.text(bi.resource)
    ]
  , Html.td [class "resource-version"]
    [ viewVersion bi.version
    ]
  ]

viewBuildOutputs : Model -> Concourse.BuildResourcesOutput -> Html Msg
viewBuildOutputs model bo =
  Html.tr [class "mbs pas resource fl clearfix"]
  [ Html.td [class "resource-name mrm"]
    [ Html.text(bo.resource)
    ]
  , Html.td [class "resource-version"]
    [ viewVersion bo.version
    ]
  ]

viewVersion : Concourse.Version -> Html Msg
viewVersion version =
  DictView.view << Dict.map (\_ s -> Html.text s) <|
    version

triggerBuild : Concourse.JobIdentifier -> Cmd Msg
triggerBuild job =
  Cmd.map BuildTriggered << Task.perform Err Ok <|
    Concourse.Job.triggerBuild job

fetchJobBuilds : Concourse.JobIdentifier -> Maybe Concourse.Pagination.Page -> Cmd Msg
fetchJobBuilds jobIdentifier page =
  Cmd.map JobBuildsFetched << Task.perform Err Ok <|
    Concourse.Build.fetchJobBuilds jobIdentifier page

fetchJob : Concourse.JobIdentifier -> Cmd Msg
fetchJob jobIdentifier =
  Cmd.map JobFetched << Task.perform Err Ok <|
    Concourse.Job.fetchJob jobIdentifier

fetchBuildResources : Concourse.BuildId -> Cmd Msg
fetchBuildResources buildIdentifier =
  Cmd.map (BuildResourcesFetched buildIdentifier) << Task.perform Err Ok <|
    Concourse.BuildResources.fetch buildIdentifier

paginationParam : Page -> String
paginationParam page =
  case page.direction of
    Concourse.Pagination.Since i -> "since=" ++ toString i
    Concourse.Pagination.Until i -> "until=" ++ toString i
    Concourse.Pagination.From i -> "from=" ++ toString i
    Concourse.Pagination.To i -> "to=" ++ toString i

pauseJob : Concourse.JobIdentifier -> Cmd Msg
pauseJob jobIdentifier =
  Cmd.map PausedToggled << Task.perform Err Ok <|
    Concourse.Job.pause jobIdentifier


unpauseJob : Concourse.JobIdentifier -> Cmd Msg
unpauseJob jobIdentifier =
  Cmd.map PausedToggled << Task.perform Err Ok <|
    Concourse.Job.unpause jobIdentifier

getCurrentTime : Cmd Msg
getCurrentTime =
  Task.perform (always Noop) ClockTick Time.now

subscriptions : Model -> Sub Msg
subscriptions model =
  Sub.batch
    [ Time.every (5 * Time.second) SubscriptionTick
    , Time.every (1 * Time.second) ClockTick
    ]
