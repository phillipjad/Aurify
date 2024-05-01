import { Link } from "react-router-dom";
import { ButtonGroup, Row } from "react-bootstrap";

function Splash() {
  return (
    <Row
      className="
        drop-shadow-2xl
        shadow-inner
        d-flex
        justify-content-center
        align-items-center
        text-center
        mx-auto
        w-72
        h-80
        rounded-lg
        bg-opacity-25
        bg-slate-100
      "
    >
      <ButtonGroup
        role="group"
        className="
          align-content-center
          justify-content-center
          flex-wrap
        "
        vertical
      >
        <Link className="w-100 mb-5" to="/playlists">
          <button
            className="
              w-60
              p-3
              text-3xl
              text-white
              rounded-full
              drop-shadow-lg
              border
              bg-green-600
              bg-opacity-75
              hover:bg-opacity-100
              hover:drop-shadow-xl
            "
          >Playlists</button>
        </Link>
        <Link className="w-100" to="/about">
          <button
            className="
            w-60
            p-3
            text-3xl
            text-white
            rounded-full
            drop-shadow-lg
            border
            bg-green-600
            bg-opacity-75
            hover:bg-opacity-100
            hover:drop-shadow-xl
          "
          >About Us</button>
        </Link>
      </ButtonGroup>
    </Row>
  )
}

export default Splash;