# Examples

## ubiq

[ubiq.html](ubiq.html) is the page `ubiq html` renders for this repository: the root
[GLOSSARY.md](https://github.com/stefaneg/ubiq/blob/main/GLOSSARY.md) and the [ADRs](../README.md#learn-more)
in `docs/adr`. To render it again after changing the glossary, run this from the repository root:

```sh
ubiq html -out docs/examples/ubiq.html
```

## course-video-manager

[mattpocock/course-video-manager](https://github.com/mattpocock/course-video-manager) keeps a larger single-context
glossary: one root `GLOSSARY.md` and its ADRs in `docs/adr`. The project has no license, so this repository does not
include a rendered page. To see the page, clone the project and render it:

```sh
git clone https://github.com/mattpocock/course-video-manager
ubiq html -root course-video-manager -out course-video-manager.html
```

Then open `course-video-manager.html` in a browser.
