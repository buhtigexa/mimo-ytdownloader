# mimo-ytdownloader

Descarga audio MP3 desde una URL, un archivo de texto con una URL por línea o un CSV. En un CSV con columnas `artist` y `title`, busca las URL que falten y las agrega al mismo archivo.

## Docker

Desde la raíz del repositorio, construye la imagen:

```bash
docker compose build mimo-ytdownloader
```

Indica la carpeta que contiene tus archivos y ejecuta con una URL directa, una lista de URL o un CSV:

```bash
export MIMO_YTDOWNLOADER_DATA_DIR="/ruta/a/tus/archivos"
docker compose run --rm mimo-ytdownloader "https://www.youtube.com/watch?v=ID_DEL_VIDEO"
docker compose run --rm mimo-ytdownloader /data/urls.txt
docker compose run --rm mimo-ytdownloader /data/archivo.csv
```

`MIMO_YTDOWNLOADER_DATA_DIR` es la carpeta del equipo donde están las listas y los CSV. Dentro del contenedor aparece como `/data`. Si omites la variable, se usa `./data` en la raíz del repositorio.

Los archivos descargados quedan en la subcarpeta `out/` de la carpeta elegida. Las URL encontradas se guardan en el CSV original al terminar la búsqueda. El progreso y los errores se muestran en la terminal.

Se hacen 4 descargas simultáneas por ejecución y el límite configurable es 5. Cada descarga usa una transferencia de fragmento a la vez.
