# Local translation models on Apple silicon: speed and quality against Apple's engine

Kept for issue #13 (local translation models: download, activate and run on the Mac) and issue #10 (System engine speed). Measured on 2026-09-30.

**What this is.** Our own measurements, not research from the web. The question: can Kai run small translation models locally on the Mac's GPU, and how do they compare with Apple's Translation framework (the System engine) on speed and quality? The web research that prompted it is in `apple-translation-speed-grok-2026-09-30.md`.

**Reliability.** Speeds are measured and repeatable (three timed runs each, medians, agreeing within a few percent). Quality is one passage in one language pair, read by Claude, not judged by a native speaker; treat the quality notes as a first impression, not a verdict.

## Setup
- **Mac:** Apple M1 Max, 32 GB, macOS 27. Idle apart from the test (nothing else running on the GPU).
- **Runtime for the local models:** MLX 0.32.3 with `mlx-lm` 0.31.3, running on the GPU (`Device(gpu, 0)`). Python 3.13 in a throwaway virtual environment.
- **Models** (4-bit builds from the `mlx-community` organisation on Hugging Face):
  - `HY-MT1.5-1.8B-4bit` (Tencent Hunyuan translation model, 1.8B parameters)
  - `translategemma-4b-it-4bit` (Google TranslateGemma, 4B)
  - `HY-MT1.5-7B-4bit` (Tencent, 7B)
  - `translategemma-12b-it-4bit` (Google, 12B)
- **Apple's engine:** `TranslationSession(installedSource:target:preferredStrategy:)` from a standalone Swift program, with `.lowLatency` (traditional models) and `.highFidelity` (Apple Intelligence models, what Kai uses today). The English and Spanish (Spain) language packs were downloaded so the fast mode could run; the Spain pack also serves Spanish (Mexico).
- **Direction:** English to Spanish (Mexico) (`es-MX`).
- **Texts:** two real spoken-English passages, 1,260 and 2,790 characters (a conference keynote opening). The texts are not reproduced here.
- **Method:** one untimed warm-up, then three timed runs per text, median reported. Time is wall-clock from the request to the last token. Local models use the prompt format each model expects: TranslateGemma through its chat template with `source_lang_code` and `target_lang_code`, with `<end_of_turn>` added as a stop token (without it the model keeps emitting filler until the token cap and the timing is wrong); HY-MT with "Translate the following segment into Mexican Spanish, without additional explanation."

## Results

| Engine | 1,260 chars | 2,790 chars | Per character | Time to first token | Peak memory |
|---|---|---|---|---|---|
| Apple `.lowLatency` | 0.68 s | 1.48 s | 0.53 ms | | small |
| HY-MT 1.5 1.8B | 2.36 s | 5.03 s | 1.8 ms | 0.28 to 0.47 s | 1.5 to 1.8 GB |
| TranslateGemma 4B | 3.85 s | 8.20 s | 2.9 to 3.1 ms | 0.68 to 1.07 s | 2.8 to 3.0 GB |
| HY-MT 1.5 7B | 6.41 s | 16.4 s | 5.1 to 5.9 ms | 0.92 to 2.11 s | 4.7 to 4.9 GB |
| Apple `.highFidelity` (Kai's default today) | 9.51 s | 21.3 s | 7.5 to 7.6 ms | | |
| TranslateGemma 12B | 12.1 s | 28.7 s | 9.6 to 10.3 ms | 2.1 to 3.9 s | 7.3 to 7.6 GB |

Individual runs (seconds):

| Engine | 1,260 chars | 2,790 chars |
|---|---|---|
| Apple `.lowLatency` | 0.93, 0.68, 0.67 | 1.52, 1.48, 1.48 |
| HY-MT 1.8B | 2.37, 2.36, 2.36 | 5.03, 5.01, 5.03 |
| TranslateGemma 4B | 3.77, 3.85, 3.92 | 8.24, 8.20, 8.20 |
| HY-MT 7B | 6.23, 6.41, 6.64 | 15.8, 16.5, 16.4 |
| Apple `.highFidelity` | 9.69, 9.42, 9.51 | 21.2, 21.3, 21.5 |
| TranslateGemma 12B | 11.9, 12.1, 12.5 | 28.2, 29.0, 28.7 |

**Scaling.** Time is linear in length (per-character time barely changes from 1,260 to 2,790 characters), so a 54,000-character text (the case in #10) would take roughly: Apple fast 29 s; HY-MT 1.8B 1.6 min; TranslateGemma 4B 2.6 min; HY-MT 7B 5.3 min; Apple default 6.9 min (about 10 min measured in the app under load); TranslateGemma 12B 9.3 min.

**Other findings.**
- **Apple serializes requests.** Two or four translations at once, on the same pair or on different pairs, gave 0.93 to 1.15 times the sequential speed. Apple's batch calls (`translations(from:)` and `translate(batch:)`) gave no speedup either (24.1 s against 23.2 s for four separate 1,500-character calls) and returned identical text. Details in issue #10.
- **Kai's bridge adds no cost.** The same 1,500-character text took 6.5 s through Kai's Swift bridge and about 5.8 s in a standalone program.
- **Apple's default is the slow mode.** Apple added a strategy switch in macOS 26.4; Kai sets none, so it runs the Apple Intelligence "high fidelity" models at about 8 ms per character. The fast mode is about 12 times quicker but needs the traditional language packs to be installed.
- **The earlier single-run table** (in issue #13's history) was slower for three models because the first run included kernel compile time and, for the 4B, a noisy second run. The three-run medians above replace it.
- **Grok's figure did not hold:** it reported TranslateGemma at about 22 ms per character on an M2 Max; here it is about 3 ms (4B) and 10 ms (12B).

## Quality: the same 1,260-character passage, English to Spanish (Mexico)

Source: the opening of a conference keynote, spoken English with a fragment ("Is proud to be the number one contributor to Drupal"). What each engine wrote (last timed run):

**Apple `.lowLatency`**

Buenos días a todos. Bienvenido de vuelta de la pausa para el café. Espero que todos estén completamente cafeinados y listos para ir. Bienvenido a DrupalCon Rotterdam. Soy el líder del equipo de marca y experiencia global en Acquia. Y hoy estoy aquí para hablar rápidamente de una palabra que recorre toda la comunidad. Se enorgullece de ser el colaborador número uno de Drupal. Lo hemos estado durante mucho tiempo y estamos muy orgullosos de eso. Pero la mayor parte de eso aparece de maneras que puedes medir: confirmaciones de código, mantenedores principales, innovación de productos, miles de horas y por personas que aman el proyecto tanto como todos ustedes. Y vamos a seguir haciendo todo eso. Pero este año nos hemos hecho una simple pregunta: ¿cómo podría ser otra contribución? Si has pasado por nuestra cabina, has visto la respuesta. Está hecho de cartón. Y en la pared, tuvimos a un artista dibujando a mano una simple oración, ya que gastamos menos en nuestro stand este año para poder donar cincuenta mil dólares a Drupal. Ahora es un poco temprano en la mañana para que pueda hacer los cálculos para convertir eso a euros, pero dejaré eso a todos ustedes. No hicimos esto por aplausos. Lo hicimos porque es lo correcto, y esperamos que encienda algo dentro de toda la comunidad.

**Apple `.highFidelity`**

Buenos días a todos. Bienvenidos de nuevo del descanso para tomar café. Espero que todos estén completamente cargados de cafeína y listos para comenzar. Bienvenidos a DrupalCon Rotterdam. Soy el líder del equipo de marca y experiencia global en Acquia. Y estoy aquí hoy para hablar rápidamente de una palabra que recorre toda la comunidad. Estoy orgulloso de ser el principal colaborador de Drupal. Lo hemos sido durante mucho tiempo y estamos muy orgullosos de ello. Pero la mayor parte de eso se refleja de maneras que se pueden medir: commits de código, mantenedores del núcleo, innovación del producto, miles de horas y por personas que aman el proyecto tanto como ustedes. Y vamos a seguir haciendo todo eso. Pero este año nos hicimos una pregunta sencilla: ¿cómo podría verse otra vez la contribución? Si han pasado por nuestro stand, han visto la respuesta. Está hecho de cartón. Y en la pared, un artista dibujó a mano una sola frase sencilla mientras gastábamos menos en nuestro stand este año para poder donar cincuenta mil dólares a Drupal. Ahora es un poco temprano en la mañana para que yo pueda hacer los cálculos para convertir eso a euros, pero lo dejo a ustedes. No lo hicimos por aplausos. Lo hicimos porque es lo correcto que hacer, y esperamos que despierte algo dentro de toda la comunidad.

**HY-MT 1.5 1.8B**

Buenos días a todos. Bienvenidos de nuevo después de la pausa para el café. Espero que todos estén completamente hidratados y listos para continuar con el trabajo. Bienvenidos a DrupalCon Rotterdam. Soy el líder del equipo de marca y experiencia global en Acquia. Hoy quiero hablar rápidamente sobre una palabra que está presente en toda la comunidad. Estamos orgullosos de ser los principales contribuyentes de Drupal. Lo hemos hecho durante mucho tiempo, y estamos muy orgullosos de ello. Pero todo esto se refleja en cosas que se pueden medir: commits de código, mantenedores de código base, innovación en productos, miles de horas de trabajo, y también en las personas que aman el proyecto tanto como ustedes. Y seguiremos haciendo todo esto. Pero este año nos preguntamos una pregunta simple: ¿cómo podría ser la contribución de Drupal? Si han pasado por nuestro stand, habrán visto la respuesta. Es una pieza hecha de cartón. Y en la pared, un artista dibujó una frase simple. Este año dedicamos menos dinero al stand, para poder donar cincuenta mil dólares a Drupal. Ahora es temprano para calcular el valor en euros, pero dejaré ese trabajo para ustedes. No lo hicimos por el aprecio de la gente. Lo hicimos porque es lo correcto, y esperamos que esto inspire algo en toda la comunidad.

**TranslateGemma 4B**

Buenos días a todos. Bienvenidos después de la pausa para el café. Espero que estén completamente energizados y listos para empezar. Bienvenidos a DrupalCon Rotterdam. Soy el líder del equipo de marca y experiencia global en Acquia. Y hoy estoy aquí para hablar brevemente sobre una palabra que define a toda la comunidad. Estamos orgullosos de ser los principales contribuyentes de Drupal, y lo hemos sido durante mucho tiempo, y estamos muy orgullosos de ello. Pero la mayor parte de esto se manifiesta en formas que podemos medir: commits de código, mantenedores centrales, innovación de productos, miles de horas de trabajo y gracias a personas que aman el proyecto tanto como ustedes. Y seguiremos haciendo todo esto. Pero este año, nos preguntamos: ¿qué más podría significar la contribución? Si han pasado por nuestro stand, han visto la respuesta. Está hecho de cartón. Y en la pared, teníamos a un artista que dibujó una frase sencilla, mientras que este año hemos reducido los gastos de nuestro stand para donar 50.000 dólares a Drupal. Ahora, para mí es un poco pronto por la mañana para hacer los cálculos y convertirlo a euros, pero los dejo a ustedes. No hicimos esto por aplausos. Lo hicimos porque es lo correcto, y esperamos que esto inspire algo en toda la comunidad.

**HY-MT 1.5 7B**

Buenos días a todos. Bienvenidos de vuelta después del descanso para tomar café. Espero que todos estén bien cargados de cafeína y listos para continuar. Bienvenidos a DrupalCon Rotterdam. Soy el líder del equipo de marca y experiencia global en Acquia, y hoy estoy aquí para hablar rápidamente sobre una palabra que recorre toda la comunidad: estamos orgullosos de ser el principal contribuyente a Drupal. Lo hemos sido durante mucho tiempo, y nos enorgullece mucho de ello. Pero la mayor parte de nuestro aporte se manifiesta de formas que se pueden medir: contribuciones de código, participación en el mantenimiento del proyecto, innovación en los productos, miles de horas de trabajo, y, por supuesto, personas que aman este proyecto tanto como ustedes. Y seguiremos haciendo todo esto. Pero este año nos hicimos una pregunta sencilla: ¿cómo podría ser aún mayor nuestro aporte? Si han pasado por nuestro stand, habrán visto la respuesta: está hecho de cartón. En la pared, un artista dibujó una frase sencilla; este año gastamos menos en nuestro stand para poder donar cincuenta mil dólares a Drupal. Todavía es temprano por la mañana para hacer los cálculos y convertir esa cantidad en euros, pero eso lo dejamos en sus manos. No lo hicimos por recibir aplausos, sino porque es lo correcto que había que hacer. Y esperamos que esto despierte algo en toda la comunidad.

**TranslateGemma 12B**

Buenos días a todos. Bienvenidos de nuevo del receso para el café. Espero que todos estén completamente revitalizados y listos para comenzar. Les doy la bienvenida a DrupalCon Rotterdam. Soy el líder del equipo de marca y experiencia global en Acquia. Y estoy aquí hoy para hablar brevemente sobre una palabra que recorre toda la comunidad: orgullo. Nos enorgullece ser el principal contribuyente a Drupal. Lo hemos sido durante mucho tiempo y estamos muy orgullosos de ello. Pero gran parte de esto se manifiesta de maneras que se pueden medir: contribuciones de código, mantenedores principales, innovación de productos, miles de horas, y por personas que sienten una pasión por el proyecto, tal como todos ustedes. Y vamos a seguir haciendo todo eso. Pero este año nos hicimos una pregunta sencilla: ¿cómo podría ser la contribución de otra manera? Si pasaron por nuestro stand, ya vieron la respuesta. Está hecha de cartón. Y en la pared, un artista dibujó a mano una frase sencilla, ya que este año destinamos menos recursos a nuestro stand para poder donar cincuenta mil dólares a Drupal. Quizás es un poco temprano en la mañana para que yo pueda hacer los cálculos para convertir eso a euros, pero les dejo esa tarea a ustedes. No hicimos esto para recibir aplausos. Lo hicimos porque es lo correcto, y esperamos que esto inspire a toda la comunidad.

First-impression notes (Claude, not a native speaker):
- **Apple fast mode** is the roughest: it dropped the subject of the fragment ("Se enorgullece de ser el colaborador número uno"), used the singular "Bienvenido" for a room, wrote "Lo hemos estado" for "we have been", and mixed "puedes" with "ustedes".
- **Apple default** reads naturally and stays in one register.
- **HY-MT 1.8B** is fast and mostly good, with two slips: "hidratados" for "caffeinated" and "por el aprecio de la gente" for "for applause".
- **TranslateGemma 4B** is solid and faithful ("commits de código", "han visto la respuesta"), with one small wobble ("gracias a personas que aman el proyecto"). Better than Apple's fast mode and about as good as its default.
- **HY-MT 7B** is the most natural prose but rephrases freely (adds "por supuesto", merges sentences), so it is less literal.
- **TranslateGemma 12B** is very good and faithful, and it caught that the "one word" is pride; too slow for long text.

## Where the models are stored
The four models are in the Zot registry on Jupiter as `translation/hy-mt1.5-1.8b`, `translation/translategemma-4b-it`, `translation/hy-mt1.5-7b` and `translation/translategemma-12b-it`, tag `4bit-mlx` (MLX safetensors, tokenizer and config files; 1.0, 2.2, 4.2 and 6.7 GB), OCI artifact type `application/vnd.mlx.model.v1`. The 1.8B and 4B were pulled back from the registry for the timed runs to confirm the round trip.

## How to repeat this
1. Install: `uv venv --python 3.13 .venv` and `uv pip install mlx-lm`.
2. Get a model: `hf download mlx-community/translategemma-4b-it-4bit`, or pull it from the registry with `oras pull`.
3. Time it with `mlx_lm.stream_generate`, one warm-up then three runs, stopping at `<end_of_turn>` for TranslateGemma. The prompt and loop are about twenty lines: build the chat message (TranslateGemma: a content item with `source_lang_code`, `target_lang_code` and `text`; HY-MT: the instruction above plus the text), apply the tokenizer's chat template, stream tokens, record the time to the first token and to the last.
4. Apple's side: a small Swift program that creates `TranslationSession(installedSource:target:preferredStrategy:)`, warms it with one call, then times `session.translate(text)` three times per strategy.

## Not measured yet (open in #13)
- llama.cpp with Metal on the same texts, and a Marian or NLLB model (no MLX build exists, and CTranslate2 has no Apple GPU support).
- Other language pairs: Spanish to English and a CJK pair, where quality and speed may differ a lot.
- A long text run end to end (54,000 characters) to check for slowdown over time (memory or heat).
- Quality judged by a native speaker, and on more than one passage.
