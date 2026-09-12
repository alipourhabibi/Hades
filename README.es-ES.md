

# Hades

Un registro de esquemas de código abierto compatible con [Buf](https://github.com/bufbuild/buf) para administrar y versionar definiciones de Protocol Buffer.

<table width="100%">
  <tr>
    <td width="50%" align="center">
      <img src=".github/assets/overview.png" alt="Resumen del módulo" width="100%"/>
      <br/><sub><b>Resumen del módulo</b> (último commit, buf.yaml y visibilidad)</sub>
    </td>
    <td width="50%" align="center">
      <img src=".github/assets/sdk.png" alt="SDKs generados" width="100%"/>
      <br/><sub><b>SDKs generados</b> (comandos de instalación por lenguaje con selector de versión)</sub>
    </td>
  </tr>
</table>

<video src="https://github.com/user-attachments/assets/a9758133-1611-4e75-b801-a9b55e341319" autoplay loop muted playsinline width="100%"></video>
<p align="center"><sub><b>buf push → commit visible en el registro</b></sub></p>

## Inicio rápido

**Ejecutar sin dependencias externas (SQLite + sistema de archivos local):**

```bash
go run ./cmd/hades serve --config config/dev.yaml
```

El archivo predeterminado `config/dev.yaml` utiliza SQLite para los metadatos y el disco local para git y los artefactos. No se requiere Docker.

**Ejecutar con Postgres:**

```bash
docker compose -f development/docker-compose-minimal.yaml up -d
make migrate-up
go run ./cmd/hades serve --config config/dev.yaml
```

Consulta [DEVELOPMENT.md](DEVELOPMENT.md) para obtener la guía completa de desarrollo.

## Almacenamiento

Los backends de almacenamiento son configurables e intercambiables:

| Capa | Predeterminado | Alternativas |
|---|---|---|
| Metadatos | SQLite | PostgreSQL |
| Git | go-git (local) | Gitaly |
| Artefactos | Disco local | Gitaly / S3 / MinIO |
| Caché | En memoria | Redis |

## Uso de la CLI de Buf

```bash
# Autenticarse (escribe en ~/.netrc)
buf registry login your.domain.com

# Publicar un módulo
buf push

# Resolver dependencias desde tu registro
buf dep update

# Generar código
buf generate
```

Consulta `development/protos/simpleproject` para ver un ejemplo funcional.

## Documentación

- [DEVELOPMENT.md](DEVELOPMENT.md): configuración local, SQLite vs Postgres, pruebas, referencia de configuración
- [DEPLOYMENT.md](DEPLOYMENT.md): despliegue en producción, TLS, Gitaly, S3, observabilidad

## Contribuciones

Hades está en desarrollo activo. La arquitectura es intencional y está en constante mejora. Si identificas un enfoque mejor, abre un issue o PR. Las contribuciones son bienvenidas.

## Licencia

Apache 2.0. Consulta [LICENSE](LICENSE).
