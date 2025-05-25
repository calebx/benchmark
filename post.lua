wrk.method = "POST"
wrk.headers["Content-Type"] = "application/json"

-- Alphanumeric characters
local charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

-- Function to generate a random string
local function randomXID(length)
  local result = {}
  for i = 1, length do
    local rand = math.random(#charset)
    result[i] = charset:sub(rand, rand)
  end
  return table.concat(result)
end

-- Called once at startup
math.randomseed(os.time())

-- Called before each request
request = function()
  local xid = randomXID(100)
  local body = string.format('{"xid":"%s"}', xid)
  return wrk.format(nil, nil, nil, body)
end