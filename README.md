# ginPrismaApp


### API testing

#### Register
```bash
curl -X POST http://localhost:8080/api/register \
-H "Content-Type: application/json" \
-d '{"username":"exampleUser", "password":"examplePass", "email":"user@example.com", "age":30}'
```


#### Login
```bash
curl -X POST http://localhost:8080/api/login \
-H "Content-Type: application/json" \
-d '{"email":"user@example.com", "password":"examplePass"}'
```

#### Profile

```bash
curl -X GET http://localhost:8080/api/profile \
-H "Authorization: Bearer $TOKEN"
```

#### Upload

```bash
curl -X POST http://localhost:8080/api/video/upload \
  -H "Authorization: Bearer $JWT_TOKEN" \
  -F "file=@/path/to/awesome_video.mp4;type=video/mp4"
```
