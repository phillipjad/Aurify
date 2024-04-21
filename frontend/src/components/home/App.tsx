import { useEffect, useState } from 'react'
import { Container } from 'react-bootstrap'
import { Outlet } from 'react-router-dom'

function App() {
  const [token, setToken] = useState("");
  

  useEffect(()=>{
    console.log(token);
  }, [token])

  return (
    <Container fluid>
      <h1 
        style={{userSelect: 'none'}}
        className='text-8xl text-green-500 mt-4 text-center kaushan-script-regular'>
          Aurify
      </h1>
      <main 
        id='current-route'
        className="d-flex mt-5 container-fluid justify-content-center"
      >
        <Outlet context={setToken}/>
      </main>
    </Container>
  )
}

export default App
